package client

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const v1Cfg = `apex: dev.specter.dev:1234
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
`

const bare = `version: 2
apex: dev.specter.dev:1234
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
    proxyHeaderTimeout: 60s
    proxyHeaderHost: blah.com
`

const testPrivateKey = `MC4CAQAwBQYDK2VwBCIEIFXA98L8HvJQxzyqYosZxyaX/G1vfJ4TeSP0E+N0FIfj`
const registered = `version: 2
apex: dev.specter.dev:1234
privKey: |
  -----BEGIN PRIVATE KEY-----
  ` + testPrivateKey + `
  -----END PRIVATE KEY-----
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
  - target: http://127.0.0.1:2813
    hostname: http.dev.specter.dev
`

const namedPipe = `version: 2
apex: dev.specter.dev:1234
tunnels:
  - target: \\.\pipe\something
    hostname: pipe
`

const unixSocket = `version: 2
apex: dev.specter.dev:1234
tunnels:
  - target: unix:///tmp/nginx.sock
    hostname: unix
`

func TestConfig(t *testing.T) {
	as := require.New(t)

	bareFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(bareFile.Name())

	_, err = bareFile.WriteString(bare)
	as.NoError(err)
	as.NoError(bareFile.Close())

	bareCfg, err := NewConfig(bareFile.Name())
	as.NoError(err)
	as.Equal("dev.specter.dev:1234", bareCfg.Apex)
	as.NotEmpty(bareCfg.PrivKey)
	bareCfg.buildRouter()
	as.Equal(1, bareCfg.router.Len())
	route, ok := bareCfg.router.Load("tcp.dev.specter.dev")
	as.True(ok)
	as.Equal(time.Second*60, route.proxyHeaderReadTimeout)
	as.Equal("blah.com", route.proxyHeaderHost)

	regFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(regFile.Name())

	_, err = regFile.WriteString(registered)
	as.NoError(err)
	as.NoError(regFile.Close())

	regCfg, err := NewConfig(regFile.Name())
	as.NoError(err)
	as.Equal("dev.specter.dev:1234", regCfg.Apex)
	as.Contains(regCfg.PrivKey, testPrivateKey)
	regCfg.buildRouter()
	as.Equal(2, regCfg.router.Len())

	as.NoError(err)
	regCfg.Tunnels = append(regCfg.Tunnels, Tunnel{
		Hostname: "https.dev.specter.dev",
		Target:   "https://127.0.0.1",
	})
	as.NoError(regCfg.writeFile())

	regCfg, err = NewConfig(regFile.Name())
	as.NoError(err)
	regCfg.buildRouter()
	as.Equal(3, regCfg.router.Len())
}

func TestPipeOrSocket(t *testing.T) {
	as := require.New(t)

	pipeFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(pipeFile.Name())

	_, err = pipeFile.WriteString(namedPipe)
	as.NoError(err)
	as.NoError(pipeFile.Close())

	pipeCfg, err := NewConfig(pipeFile.Name())
	if runtime.GOOS == "windows" {
		as.NoError(err)
		pipeCfg.buildRouter()
		as.Equal(1, pipeCfg.router.Len())
		r, ok := pipeCfg.router.Load("pipe")
		as.True(ok)
		u := r.parsed
		as.Equal("winio", u.Scheme)
		as.Equal("\\\\.\\pipe\\something", u.Path)
	} else {
		as.Error(err)
	}

	sockFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(sockFile.Name())

	_, err = sockFile.WriteString(unixSocket)
	as.NoError(err)
	as.NoError(sockFile.Close())

	sockCfg, err := NewConfig(sockFile.Name())
	if runtime.GOOS == "windows" {
		as.Error(err)
	} else {
		as.NoError(err)
		sockCfg.buildRouter()
		as.Equal(1, sockCfg.router.Len())
		r, ok := sockCfg.router.Load("unix")
		as.True(ok)
		u := r.parsed
		as.Equal("unix", u.Scheme)
		as.Equal("/tmp/nginx.sock", u.Path)
	}
}

func TestRebuild(t *testing.T) {
	as := require.New(t)

	regFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(regFile.Name())

	_, err = regFile.WriteString(registered)
	as.NoError(err)
	as.NoError(regFile.Close())

	regCfg, err := NewConfig(regFile.Name())
	as.NoError(err)
	as.NoError(regCfg.validate())
	regCfg.buildRouter()
	as.Equal(2, regCfg.router.Len())

	drop := 1
	dropTunnel := regCfg.Tunnels[drop]

	// drop
	_, ok := regCfg.router.Load(dropTunnel.Hostname)
	as.True(ok)
	regCfg.Tunnels = append(regCfg.Tunnels[:drop], regCfg.Tunnels[drop+1:]...)
	as.NoError(regCfg.validate())
	regCfg.buildRouter(dropTunnel)
	as.Equal(1, regCfg.router.Len())
	_, ok = regCfg.router.Load(dropTunnel.Hostname)
	as.False(ok)

	// update
	prev := regCfg.Tunnels[0]
	r, ok := regCfg.router.Load(prev.Hostname)
	as.True(ok)
	as.Equal(prev.Target, r.parsed.String())
	newTarget := "http://127.0.0.1:8080"
	regCfg.Tunnels[0].Target = newTarget
	as.NoError(regCfg.validate())
	regCfg.buildRouter(prev)
	as.Equal(1, regCfg.router.Len())
	r, ok = regCfg.router.Load(prev.Hostname)
	as.True(ok)
	as.Equal(newTarget, r.parsed.String())
}

func TestV1Config(t *testing.T) {
	as := require.New(t)

	v1File, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(v1File.Name())

	_, err = v1File.WriteString(v1Cfg)
	as.NoError(err)
	as.NoError(v1File.Close())

	_, err = NewConfig(v1File.Name())
	as.Error(err)
}
