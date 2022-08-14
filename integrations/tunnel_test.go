package integrations

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
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
	serverPort   = 21948
	serverApex   = "dev.con.nect.sh"
	yamlTemplate = `apex: 127.0.0.1:%d
tunnels:
  - target: %s
  - target: %s
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
		Metadata: make(map[string]interface{}),
	}, observedLogs
}

func TestTunnel(t *testing.T) {
	if os.Getenv("GO_RUN_INTEGRATION") == "" {
		t.Skip("skipping integration tests")
	}

	as := require.New(t)

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testBody))
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	as.NoError(err)
	defer listener.Close()

	tcpTarget := listener.Addr().String()

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	_, err = file.WriteString(fmt.Sprintf(yamlTemplate, serverPort, ts.URL, fmt.Sprintf("tcp://%s", tcpTarget)))
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
	case <-time.After(time.Second * 2):
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
		cApp.Metadata["apexOverride"] = serverApex
		if err := cApp.RunContext(ctx, clientArgs); err != nil {
			as.NoError(err)
		}
		close(clientReturn)
	}()

	select {
	case <-clientReturn:
		as.FailNow("client returned unexpectedly")
	case <-time.After(time.Second * 2):
	}

	hostMap := make(map[string]string)
	clientLogs := cLogs.All()
	for _, l := range clientLogs {
		if !strings.Contains(l.Message, "published") {
			continue
		}
		var hostname string
		var proto string
		for _, f := range l.Context {
			switch f.Key {
			case "hostname":
				hostname = f.String
			case "target":
				if strings.Contains(f.String, "http") {
					proto = "http"
				}
				if strings.Contains(f.String, "tcp") {
					proto = "tcp"
				}
			}
		}
		if hostname != "" && proto != "" {
			hostMap[proto] = hostname
		}
	}

	as.Equal(2, len(hostMap), "hostname for tunnels not found in client log")

	t.Logf("Found hostnames %v\n", hostMap)

	// ====== HTTP TUNNEL ======

	req, err := http.NewRequest("GET", fmt.Sprintf("https://%s:%d/", hostMap["http"], serverPort), nil)
	as.NoError(err)

	cfg := &tls.Config{
		ServerName:         hostMap["http"],
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig:   cfg,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialer := &tls.Dialer{
					Config: cfg,
				}
				return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", serverPort))
			},
		},
		Timeout: time.Second * 2,
	}
	resp, err := httpClient.Do(req)
	as.NoError(err)
	defer resp.Body.Close()

	as.True(resp.ProtoAtLeast(2, 0))

	var buf bytes.Buffer
	_, err = buf.ReadFrom(resp.Body)
	as.NoError(err)
	as.Equal(testBody, buf.String())

	// ====== TCP TUNNEL ======

	connectArgs := []string{
		"specter",
		"client",
		"--insecure",
		"connect",
		fmt.Sprintf("127.0.0.1:%d", serverPort),
	}

	connectReturn := make(chan struct{})

	xApp, xLogs := compileApp(client.Cmd)
	go func() {
		xApp.Metadata["connectOverride"] = hostMap["tcp"]
		if err := xApp.RunContext(ctx, connectArgs); err != nil {
			as.NoError(err)
		}
		close(connectReturn)
	}()

	select {
	case <-connectReturn:
		as.FailNow("connect returned unexpectedly")
	case <-time.After(time.Second * 3):
	}

	connected := false
	connectLogs := xLogs.All()
	for _, l := range connectLogs {
		if strings.Contains(l.Message, "established") {
			connected = true
		}
	}
	as.True(connected, "expecting tunnel established")
}
