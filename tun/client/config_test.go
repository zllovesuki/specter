package client

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
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

const bareHeader = `version: 2
apex: dev.specter.dev:1234
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
    headerTimeout: 45s
    headerHost: header.com
    headerMode: hostname
`

const bareMixedHeaders = `version: 2
apex: dev.specter.dev:1234
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
    proxyHeaderTimeout: 60s
    proxyHeaderHost: old.com
    proxyHeaderMode: target
    headerTimeout: 30s
    headerHost: new.com
    headerMode: hostname
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
	as.Len(bareCfg.Tunnels, 1)
	as.Equal("tcp.dev.specter.dev", bareCfg.Tunnels[0].Hostname)
	as.Equal(time.Second*60, bareCfg.Tunnels[0].ProxyHeaderTimeout)
	as.Equal("blah.com", bareCfg.Tunnels[0].ProxyHeaderHost)

	// new header* names only
	headerFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(headerFile.Name())

	_, err = headerFile.WriteString(bareHeader)
	as.NoError(err)
	as.NoError(headerFile.Close())

	headerCfg, err := NewConfig(headerFile.Name())
	as.NoError(err)
	as.Len(headerCfg.Tunnels, 1)
	as.Equal(time.Second*45, headerCfg.Tunnels[0].ProxyHeaderTimeout)
	as.Equal("header.com", headerCfg.Tunnels[0].ProxyHeaderHost)

	// mixed legacy and new names: new should win
	mixedFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(mixedFile.Name())

	_, err = mixedFile.WriteString(bareMixedHeaders)
	as.NoError(err)
	as.NoError(mixedFile.Close())

	mixedCfg, err := NewConfig(mixedFile.Name())
	as.NoError(err)
	as.Len(mixedCfg.Tunnels, 1)
	// headerTimeout 30s should override proxyHeaderTimeout 60s
	as.Equal(time.Second*30, mixedCfg.Tunnels[0].ProxyHeaderTimeout)
	// headerHost new.com should override proxyHeaderHost old.com
	as.Equal("new.com", mixedCfg.Tunnels[0].ProxyHeaderHost)

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
	as.Len(regCfg.Tunnels, 2)

	as.NoError(err)
	regCfg.Tunnels = append(regCfg.Tunnels, Tunnel{
		Hostname: "https.dev.specter.dev",
		Target:   "https://127.0.0.1",
	})
	as.NoError(regCfg.writeFile())

	regCfg, err = NewConfig(regFile.Name())
	as.NoError(err)
	as.Len(regCfg.Tunnels, 3)
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
		as.Len(pipeCfg.Tunnels, 1)
		u := pipeCfg.Tunnels[0].parsed
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
		as.Len(sockCfg.Tunnels, 1)
		u := sockCfg.Tunnels[0].parsed
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
	f := newForwarder(zaptest.NewLogger(t))
	defer f.closeAll()
	f.setRoutes(regCfg.Tunnels)
	as.Equal(2, f.routes.Len())
	unchanged, ok := f.routes.Load(regCfg.Tunnels[0].Hostname)
	as.True(ok)
	as.NoError(regCfg.validate())
	f.setRoutes(regCfg.Tunnels)
	reused, ok := f.routes.Load(regCfg.Tunnels[0].Hostname)
	as.True(ok)
	as.Same(unchanged, reused)

	drop := 1
	dropTunnel := regCfg.Tunnels[drop]

	// drop
	_, ok = f.routes.Load(dropTunnel.Hostname)
	as.True(ok)
	regCfg.Tunnels = append(regCfg.Tunnels[:drop], regCfg.Tunnels[drop+1:]...)
	as.NoError(regCfg.validate())
	f.setRoutes(regCfg.Tunnels)
	as.Equal(1, f.routes.Len())
	_, ok = f.routes.Load(dropTunnel.Hostname)
	as.False(ok)

	// update
	prev := regCfg.Tunnels[0]
	r, ok := f.routes.Load(prev.Hostname)
	as.True(ok)
	as.Equal(prev.Target, r.parsed.String())
	newTarget := "http://127.0.0.1:8080"
	regCfg.Tunnels[0].Target = newTarget
	as.NoError(regCfg.validate())
	f.setRoutes(regCfg.Tunnels)
	as.Equal(1, f.routes.Len())
	r, ok = f.routes.Load(prev.Hostname)
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

func TestProxyHeaderModeValidation(t *testing.T) {
	as := require.New(t)

	// invalid mode
	cfg := &Config{
		Tunnels: []Tunnel{{
			Target:          "http://127.0.0.1:8080",
			ProxyHeaderMode: "invalid",
		}},
	}
	err := cfg.validate()
	as.Error(err)

	// custom mode without host should fail
	cfg = &Config{
		Tunnels: []Tunnel{{
			Target:          "http://127.0.0.1:8080",
			ProxyHeaderMode: "custom",
		}},
	}
	err = cfg.validate()
	as.Error(err)

	// target mode with pipe/unix targets should fail on all platforms
	pipeTarget := "unix:///tmp/nginx.sock"
	if runtime.GOOS == "windows" {
		pipeTarget = `\\.\\pipe\\something`
	}
	cfg = &Config{
		Tunnels: []Tunnel{{
			Target:          pipeTarget,
			ProxyHeaderMode: "target",
		}},
	}
	err = cfg.validate()
	as.Error(err)
}
