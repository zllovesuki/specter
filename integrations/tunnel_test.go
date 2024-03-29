package integrations

import (
	"bufio"
	"bytes"
	"context"
	cRand "crypto/rand"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.miragespace.co/specter/cmd/client"
	"go.miragespace.co/specter/cmd/server"
	"go.miragespace.co/specter/util/testcond"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/iangudger/memnet"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	testBinaryLength = 16
	testBody         = "yay"
	testRespBody     = "cool"
	serverApex       = "dev.con.nect.sh"
	yamlTemplate     = `version: 2
apex: 127.0.0.1:%d
tunnels:
  - target: %s
    insecure: true
  - target: %s
`
)

var (
	serverPorts     = []int{21948, 21949, 21950, 21951, 21952}
	serverHttpPorts = []int{21848, 21849, 21850, 21851, 21852}
)

type TestWsMsg struct {
	Message string
}

func compileApp(cmd *cli.Command) (*cli.App, *observer.ObservedLogs) {
	observedZapCore, observedLogs := observer.New(zap.DebugLevel)
	observedLogger := zap.New(observedZapCore)
	cmd.HideHelp = true
	return &cli.App{
		Name:        "specter",
		HideHelp:    true,
		HideVersion: true,
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

func TestIntegrationTunnel(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_TUNNEL") == "" {
		t.Skip("skipping integration tests")
	}

	seed := time.Now().Unix()
	t.Logf(" ========== Using %d as seed in this test ==========\n", seed)
	rand.Seed(seed)

	as := require.New(t)

	t.Logf("Creating HTTP server for forwarding target\n")

	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	mux := chi.NewRouter()
	mux.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(testBody))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		as.NoError(err)
		defer conn.Close()
		t := &TestWsMsg{}
		as.NoError(conn.ReadJSON(t))
		as.Equal(testBody, t.Message)
		t.Message = testRespBody
		as.NoError(conn.WriteJSON(t))
	})

	ts := httptest.NewUnstartedServer(mux)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	t.Logf("Creating TCP server for forwarding target\n")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	as.NoError(err)
	defer listener.Close()

	var (
		connectionAccepted atomic.Int32
	)
	defer func() {
		as.Equal(int32(len(serverPorts)+len(serverHttpPorts)), connectionAccepted.Load(), "tcp target should have accepted all connections")
	}()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			connectionAccepted.Add(1)
			go io.Copy(c, c)
		}
	}()

	t.Logf("Generating client config\n")

	tcpTarget := listener.Addr().String()

	file, err := os.CreateTemp("", "client")
	as.NoError(err)
	defer os.Remove(file.Name())

	_, err = file.WriteString(fmt.Sprintf(yamlTemplate, serverPorts[0], ts.URL, fmt.Sprintf("tcp://%s", tcpTarget)))
	as.NoError(err)
	as.NoError(file.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverReturnCtx, serverStopped := context.WithCancel(ctx)
	defer serverStopped()

	serverLogs := make([]*observer.ObservedLogs, len(serverPorts))
	serverArgs := make([][]string, len(serverPorts))

	for i, port := range serverPorts {
		dir, err := os.MkdirTemp("", fmt.Sprintf("integration-%d", port))
		as.NoError(err)
		defer os.RemoveAll(dir)

		serverArgs[i] = []string{
			"specter",
			"server",
			"--cert-dir",
			"../certs",
			"--data-dir",
			dir,
			"--listen",
			fmt.Sprintf("127.0.0.1:%d", port),
			"--listen-http",
			fmt.Sprintf("%d", serverHttpPorts[i]),
			"--apex",
			serverApex,
		}

		if i != 0 {
			serverArgs[i] = append(serverArgs[i], []string{
				"--join",
				fmt.Sprintf("127.0.0.1:%d", serverPorts[0]),
			}...)
		}
	}

	defer func() {
		for i, port := range serverPorts {
			logs := serverLogs[i]
			for _, entry := range logs.All() {
				t.Logf("%d: %s\n", port, entry.Message)
				// for _, x := range entry.Context {
				// 	if x.Interface != nil {
				// 		t.Logf("     %s: %v\n", x.Key, x.Interface)
				// 	} else if x.String != "" {
				// 		t.Logf("     %s: %v\n", x.Key, x.String)
				// 	} else {
				// 		t.Logf("     %s: %v\n", x.Key, x.Integer)
				// 	}
				// }
			}
		}
	}()

	t.Run("starting servers", func(t *testing.T) {
		as := require.New(t)
		for i, args := range serverArgs {
			sApp, sLogs := compileApp(server.Generate())
			serverLogs[i] = sLogs
			args := args
			go func(app *cli.App) {
				if err := app.RunContext(ctx, args); err != nil {
					as.NoError(err)
				}
				serverStopped()
			}(sApp)
		}

		as.NoError(testcond.WaitForCondition(func() bool {
			select {
			case <-serverReturnCtx.Done():
				as.FailNow("server returned unexpectedly")
				return false
			default:
				started := 0
				for _, sLogs := range serverLogs {
					serverLogs := sLogs.All()
					for _, l := range serverLogs {
						if strings.Contains(l.Message, "server started") {
							started++
						}
					}
				}
				return started == (2 * len(serverPorts))
			}
		}, time.Millisecond*100, time.Second*30), "timeout expecting specter and gateway servers started")
	})

	var hostMap map[string]string
	t.Run("starting client", func(t *testing.T) {
		as := require.New(t)

		clientReturn := make(chan struct{})
		clientArgs := []string{
			"specter",
			"client",
			"--insecure",
			"tunnel",
			"--config",
			file.Name(),
		}

		cApp, cLogs := compileApp(client.Generate())
		cApp.Metadata["apexOverride"] = serverApex
		go func() {
			if err := cApp.RunContext(ctx, clientArgs); err != nil {
				as.NoError(err)
			}
			close(clientReturn)
		}()

		t.Logf("Waiting for client to publish tunnels\n")

		as.NoError(testcond.WaitForCondition(func() bool {
			select {
			case <-clientReturn:
				as.FailNow("client returned unexpectedly")
				return false
			default:
				hostMap = make(map[string]string)
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
				return len(hostMap) == 2
			}
		}, time.Millisecond*100, time.Second*10), "timeout waiting for hostname of tunnels in client log")

		t.Logf("Found hostnames %v\n", hostMap)
	})

	t.Logf("Start integration test\n")

	for serverIndex, serverPort := range serverPorts {
		t.Run(fmt.Sprintf("with %d as endpoint", serverPort), func(t *testing.T) {
			as := require.New(t)

			req, err := http.NewRequest("GET", fmt.Sprintf("https://%s:%d/", hostMap["http"], serverPort), nil)
			as.NoError(err)
			baseCfg := &tls.Config{
				ServerName:         hostMap["http"],
				InsecureSkipVerify: true,
				NextProtos:         []string{"h2"},
			}
			httpClient := &http.Client{
				Timeout: time.Second * 2,
			}

			t.Run("HTTP over TCP", func(t *testing.T) {
				as := require.New(t)

				cfg := baseCfg.Clone()

				httpClient.Transport = &http.Transport{
					ForceAttemptHTTP2: true,
					TLSClientConfig:   cfg,
					DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						dialer := &tls.Dialer{
							Config: cfg,
						}
						return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", serverPort))
					},
				}

				resp, err := httpClient.Do(req)
				as.NoError(err)
				defer resp.Body.Close()

				as.True(resp.ProtoAtLeast(2, 0))

				var buf bytes.Buffer
				_, err = buf.ReadFrom(resp.Body)
				as.NoError(err)
				as.Equal(testBody, buf.String())
			})

			t.Run("HTTP over QUIC", func(t *testing.T) {
				as := require.New(t)

				h3Cfg := baseCfg.Clone()
				h3Cfg.NextProtos = []string{"h3"}
				httpClient.Transport = &http3.RoundTripper{
					TLSClientConfig: h3Cfg,
					Dial: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
						return quic.DialAddrEarly(ctx, fmt.Sprintf("127.0.0.1:%d", serverPort), h3Cfg, nil)
					},
				}
				resp, err := httpClient.Do(req)

				as.NoError(err)
				defer resp.Body.Close()

				as.True(resp.ProtoAtLeast(2, 0))

				var buf bytes.Buffer
				_, err = buf.ReadFrom(resp.Body)
				as.NoError(err)
				as.Equal(testBody, buf.String())
			})

			t.Run("WebSocket", func(t *testing.T) {
				as := require.New(t)

				wsCfg := baseCfg.Clone()
				wsCfg.NextProtos = []string{"http/1.1"}
				wsDialer := &websocket.Dialer{
					TLSClientConfig: wsCfg,
					NetDialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
						dialer := &tls.Dialer{
							Config: wsCfg,
						}
						return dialer.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", serverPort))
					},
				}

				wsConn, _, err := wsDialer.Dial(fmt.Sprintf("wss://%s:%d/ws", hostMap["http"], serverPort), nil)
				as.NoError(err)

				defer wsConn.Close()

				testMsg := &TestWsMsg{
					Message: testBody,
				}
				as.NoError(wsConn.WriteJSON(testMsg))
				as.NoError(wsConn.ReadJSON(testMsg))
				as.Equal(testRespBody, testMsg.Message)
			})

			t.Run("TCP Tunnel using specter client", func(t *testing.T) {
				as := require.New(t)

				connectArgs := []string{
					"specter",
					"client",
					"--insecure",
					"connect",
					fmt.Sprintf("127.0.0.1:%d", serverPort),
				}

				connectReturnCtx, connectReturn := context.WithCancel(ctx)
				defer connectReturn()

				xApp, xLogs := compileApp(client.Generate())
				xApp.Metadata["connectOverride"] = hostMap["tcp"]

				leftConn, rightConn := memnet.NewBufferedStreamConnPair()
				xApp.Metadata[client.PipeInKey] = leftConn
				xApp.Metadata[client.PipeOutKey] = leftConn

				go func() {
					if err := xApp.RunContext(ctx, connectArgs); err != nil {
						as.NoError(err)
					}
					connectReturn()
				}()

				as.NoError(testcond.WaitForCondition(func() bool {
					select {
					case <-connectReturnCtx.Done():
						as.FailNow("connect returned unexpectedly")
						return false
					default:
						connected := false
						connectLogs := xLogs.All()
						for _, l := range connectLogs {
							if strings.Contains(l.Message, "established") {
								connected = true
							}
						}
						return connected
					}
				}, time.Millisecond*100, time.Second*3), "timeout waiting for tunnel to be connected")

				write := make([]byte, testBinaryLength)
				_, err := io.ReadFull(cRand.Reader, write)
				as.NoError(err)

				_, err = rightConn.Write(write)
				as.NoError(err)

				as.NoError(rightConn.SetReadDeadline(time.Now().Add(time.Second)))
				read := make([]byte, testBinaryLength)
				n, err := rightConn.Read(read)
				as.NoError(err)
				as.Equal(testBinaryLength, n)
				as.EqualValues(write, read)
			})

			t.Run("TCP Tunnel over http connect", func(t *testing.T) {
				as := require.New(t)

				// HTTP Connect start
				dialer := &net.Dialer{
					Timeout: time.Second,
				}
				tcpConn, err := dialer.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", serverHttpPorts[serverIndex]))
				as.NoError(err)
				defer tcpConn.Close()

				proxyAddr := fmt.Sprintf("%s:%d", hostMap["tcp"], 1234)
				req := &http.Request{
					Method: http.MethodConnect,
					URL: &url.URL{
						Opaque: proxyAddr,
					},
					Host:   proxyAddr,
					Header: make(http.Header),
				}
				as.NoError(req.Write(tcpConn))
				resp, err := http.ReadResponse(bufio.NewReader(tcpConn), req)
				as.NoError(err)
				as.Equal(http.StatusOK, resp.StatusCode)
				// HTTP Connect end

				write := make([]byte, testBinaryLength)
				_, err = io.ReadFull(cRand.Reader, write)
				as.NoError(err)

				_, err = tcpConn.Write(write)
				as.NoError(err)

				as.NoError(tcpConn.SetReadDeadline(time.Now().Add(time.Second)))
				read := make([]byte, testBinaryLength)
				n, err := tcpConn.Read(read)
				as.NoError(err)
				as.Equal(testBinaryLength, n)
				as.EqualValues(write, read)
			})
		})
	}
}
