package client

import (
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

const bare = `apex: dev.specter.dev:1234
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
`

const registered = `apex: dev.specter.dev:1234
clientId: 42
token: abcdef
tunnels:
  - target: tcp://127.0.0.1:1234
    hostname: tcp.dev.specter.dev
  - target: http://127.0.0.1:2813
    hostname: http.dev.specter.dev
`

const namedPipe = `apex: dev.specter.dev:1234
tunnels:
  - target: \\.\pipe\something
    hostname: pipe
`

const unixSocket = `apex: dev.specter.dev:1234
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
	bareCfg.buildRouter()
	as.Equal(1, bareCfg.router.Len())

	regFile, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(regFile.Name())

	_, err = regFile.WriteString(registered)
	as.NoError(err)
	as.NoError(regFile.Close())

	regCfg, err := NewConfig(regFile.Name())
	as.NoError(err)
	as.Equal("dev.specter.dev:1234", regCfg.Apex)
	as.Equal("abcdef", regCfg.Token)
	as.Equal(uint64(42), regCfg.ClientID)
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
		u, ok := pipeCfg.router.Load("pipe")
		as.True(ok)
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
		u, ok := sockCfg.router.Load("unix")
		as.True(ok)
		as.Equal("unix", u.Scheme)
		as.Equal("/tmp/nginx.sock", u.Path)
	}
}
