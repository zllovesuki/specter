package client

import (
	"os"
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
