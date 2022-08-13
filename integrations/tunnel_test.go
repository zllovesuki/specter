package integrations

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"kon.nect.sh/specter/cmd/client"
	"kon.nect.sh/specter/cmd/server"
	"kon.nect.sh/specter/cmd/specter"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

const (
	httpPort     = 49192
	serverPort   = 21948
	serverApex   = "dev.con.nect.sh"
	yamlTemplate = `apex: %s:%d
tunnels:
  - target: http://127.0.0.1:%d
`
)

func compileApp(cmd *cli.Command) *cli.App {
	return &cli.App{
		Name: "specter",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "verbose",
				Value: false,
				Usage: "enable verbose logging",
			},
		},
		Commands: []*cli.Command{
			cmd,
		},
		Before: specter.ConfigLogger,
	}
}

func TestTunnel(t *testing.T) {
	if os.Getenv("GO_RUN_INTEGRATION") == "" {
		t.Skip("skipping integration tests")
	}

	as := require.New(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	_, err = file.WriteString(fmt.Sprintf(yamlTemplate, serverApex, serverPort, httpPort))
	as.NoError(err)
	as.NoError(file.Close())

	serverArgs := []string{
		"specter",
		"--verbose",
		"server",
		"--cert-dir",
		"../certs",
		"--listen",
		fmt.Sprintf("127.0.0.1:%d", serverPort),
		"--apex",
		serverApex,
	}

	clientArgs := []string{
		"specter",
		"--verbose",
		"client",
		"--insecure",
		"tunnel",
		"--config",
		file.Name(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReturn := make(chan struct{})
	clientReturn := make(chan struct{})

	go func() {
		if err := compileApp(server.Cmd).RunContext(ctx, serverArgs); err != nil {
			as.NoError(err)
		}
		close(serverReturn)
	}()

	select {
	case <-serverReturn:
		as.FailNow("server returned unexpectedly")
	case <-time.After(time.Second * 3):
	}

	go func() {
		if err := compileApp(client.Cmd).RunContext(ctx, clientArgs); err != nil {
			as.NoError(err)
		}
		close(clientReturn)
	}()

	select {
	case <-clientReturn:
		as.FailNow("client returned unexpectedly")
	case <-time.After(time.Second * 3):
	}
}
