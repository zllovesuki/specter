package integrations

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"kon.nect.sh/specter/cmd/client"
	"kon.nect.sh/specter/cmd/server"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	testBody     = "yay"
	httpPort     = 49192
	serverPort   = 21948
	serverApex   = "dev.con.nect.sh"
	yamlTemplate = `apex: 127.0.0.1:%d
tunnels:
  - target: http://127.0.0.1:%d
`
)

func compileApp(cmd *cli.Command) (*cli.App, *observer.ObservedLogs) {
	observedZapCore, observedLogs := observer.New(zap.InfoLevel)
	observedLogger := zap.New(observedZapCore)
	return &cli.App{
		Name: "specter",
		Commands: []*cli.Command{
			cmd,
		},
		Before: func(ctx *cli.Context) error {
			ctx.App.Metadata["logger"] = observedLogger
			return nil
		},
	}, observedLogs
}

func TestTunnel(t *testing.T) {
	if os.Getenv("GO_RUN_INTEGRATION") == "" {
		t.Skip("skipping integration tests")
	}

	as := require.New(t)

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	_, err = file.WriteString(fmt.Sprintf(yamlTemplate, serverPort, httpPort))
	as.NoError(err)
	as.NoError(file.Close())

	serverArgs := []string{
		"specter",
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

	sApp, sLogs := compileApp(server.Cmd)
	go func() {
		if err := sApp.RunContext(ctx, serverArgs); err != nil {
			as.NoError(err)
		}
		close(serverReturn)
	}()

	select {
	case <-serverReturn:
		as.FailNow("server returned unexpectedly")
	case <-time.After(time.Second * 3):
	}

	started := 0
	serverLogs := sLogs.All()
	for _, l := range serverLogs {
		if strings.Contains(l.Message, "server started") {
			started++
		}
	}
	as.Equal(2, started, "expecting specter and gateway servers started")

	cApp, cLogs := compileApp(client.Cmd)
	go func() {
		if err := cApp.RunContext(ctx, clientArgs); err != nil {
			as.NoError(err)
		}
		close(clientReturn)
	}()

	select {
	case <-clientReturn:
		as.FailNow("client returned unexpectedly")
	case <-time.After(time.Second * 3):
	}

	go func() {
		srv := &http.Server{
			Addr: fmt.Sprintf("127.0.0.1:%d", httpPort),
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(testBody))
			}),
		}
		srv.ListenAndServe()
	}()

	var hostname string
	clientLogs := cLogs.All()
	for _, l := range clientLogs {
		if !strings.Contains(l.Message, "published") {
			continue
		}
		for _, f := range l.Context {
			if f.Key != "hostname" {
				continue
			}
			hostname = f.String
		}
	}
	for _, l := range clientLogs {
		t.Logf("%+v\n", l)
	}
	as.NotEmpty(hostname, "hostname for tunnel not found in client log")

	t.Logf("Found hostname %s\n", hostname)

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s:%d/", hostname, serverPort), nil)
	as.NoError(err)
	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &tls.Dialer{
					Config: &tls.Config{
						ServerName:         hostname,
						InsecureSkipVerify: true,
					},
				}
				return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", serverPort))
			},
		},
		Timeout: time.Second * 3,
	}
	resp, err := client.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	as.NoError(err)
	as.Equal(testBody, buf.String())
}
