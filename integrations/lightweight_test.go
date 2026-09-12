package integrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientcmd "go.miragespace.co/specter/cmd/client"
	servercmd "go.miragespace.co/specter/cmd/server"

	"github.com/stretchr/testify/require"
	"github.com/twitchtv/twirp"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap/zaptest/observer"
)

func startIntegrationApp(t *testing.T, command *cli.Command, args ...string) (*observer.ObservedLogs, func() error) {
	t.Helper()
	app, logs := compileApp(command)
	app.Metadata["apexOverride"] = serverApex
	app.Writer = io.Discard
	// Keep peers available while cleanup stops each app in reverse start order.
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, append([]string{"specter"}, args...)) }()
	stopped := false
	stop := func() error {
		if stopped {
			return nil
		}
		stopped = true
		cancel()
		// Match the existing in-process server fixture: cancel its serving context.
		// Its Chord leave path uses that same canceled transport context, so waiting
		// for graceful ring departure here would test unrelated server shutdown.
		if command.Name == "server" {
			return nil
		}
		select {
		case err := <-done:
			return err
		case <-time.After(30 * time.Second):
			return fmt.Errorf("app did not stop")
		}
	}
	t.Cleanup(func() { require.NoError(t, stop()) })
	t.Cleanup(func() {
		if t.Failed() {
			for _, entry := range logs.All() {
				t.Log(entry.Message, entry.ContextMap())
			}
		}
	})
	return logs, stop
}

func waitIntegrationLog(t *testing.T, logs *observer.ObservedLogs, message string) {
	t.Helper()
	require.Eventually(t, func() bool { return logs.FilterMessage(message).Len() > 0 }, 30*time.Second, 20*time.Millisecond, message)
}

func startLightweightServers(t *testing.T, ports, httpPorts []int) {
	t.Helper()
	for i, port := range ports {
		// Keep physical peers visible in the existing bounded successor discovery;
		// virtual-node placement is covered by Chord's own integration tests.
		args := []string{"server", "--cert-dir", "../certs", "--data-dir", t.TempDir(), "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--listen-http", fmt.Sprint(httpPorts[i]), "--apex", serverApex, "--virtual", "1"}
		if i > 0 {
			args = append(args, "--join", fmt.Sprintf("127.0.0.1:%d", ports[0]))
		}
		logs, _ := startIntegrationApp(t, servercmd.Generate(), args...)
		waitIntegrationLog(t, logs, "specter server started")
		waitIntegrationLog(t, logs, "gateway server started")
	}
}

func fetchLightweight(t *testing.T, authority string, port int) (int, string) {
	t.Helper()
	cfg := &tls.Config{
		ServerName:         authority,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2"},
	}
	tp := &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   cfg,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&tls.Dialer{Config: cfg}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
		},
	}
	defer tp.CloseIdleConnections()
	c := &http.Client{
		Transport: tp,
		Timeout:   5 * time.Second,
	}
	resp, err := c.Get("https://" + authority + "/")
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	require.Equal(t, 2, resp.ProtoMajor)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(data)
}

func lightweightAuthority(t *testing.T, logs *observer.ObservedLogs) string {
	t.Helper()
	waitIntegrationLog(t, logs, "Tunnel ready")
	parsed, err := url.Parse(logs.FilterMessage("Tunnel ready").All()[0].ContextMap()["url"].(string))
	require.NoError(t, err)
	return parsed.Hostname()
}

func localTokenRequest(t *testing.T, port int, method, path string, body any, output any) int {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api%s", port, path), bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if len(path) >= 7 && path[:7] == "/tokens" {
		require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	}
	if output != nil {
		require.NoError(t, json.NewDecoder(resp.Body).Decode(output))
	}
	return resp.StatusCode
}

func runTokenCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	// Each stopped-client command bootstraps again; respect the existing per-IP RPC limit.
	time.Sleep(1100 * time.Millisecond)
	app, _ := compileApp(clientcmd.Generate())
	app.Metadata["apexOverride"] = serverApex
	var out bytes.Buffer
	app.Writer = &out
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	err := app.Run(ctx, append([]string{"specter", "client", "--insecure", "token"}, args...))
	return out.String(), err
}

func TestIntegrationLightweight(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_TUNNEL") == "" {
		t.Skip("set GO_INTEGRATION_TUNNEL=1")
	}
	ports := []int{21958, 21959, 21960}
	startLightweightServers(t, ports, []int{21858, 21859, 21860})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "lightweight") }))
	defer target.Close()
	t.Run("ephemeral", func(t *testing.T) {
		logs, stop := startIntegrationApp(t, clientcmd.Generate(), "client", "--insecure", "expose", "--apex", "127.0.0.1:21958", target.URL)
		authority := lightweightAuthority(t, logs)
		for _, port := range ports {
			code, body := fetchLightweight(t, authority, port)
			require.Equal(t, 200, code, body)
			require.Equal(t, "lightweight", body)
		}
		require.NoError(t, stop())
		require.Eventually(t, func() bool { code, _ := fetchLightweight(t, authority, ports[0]); return code != 200 }, 5*time.Second, 20*time.Millisecond)
	})
	t.Run("late upstream", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		listener.Close()
		logs, _ := startIntegrationApp(t, clientcmd.Generate(), "client", "--insecure", "expose", "--apex", "127.0.0.1:21958", "http://"+addr)
		authority := lightweightAuthority(t, logs)
		code, _ := fetchLightweight(t, authority, ports[0])
		require.Equal(t, 502, code)
		listener, err = net.Listen("tcp", addr)
		require.NoError(t, err)
		upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "late") })}
		defer upstream.Close()
		go upstream.Serve(listener)
		code, body := fetchLightweight(t, authority, ports[0])
		require.Equal(t, 200, code, body)
		require.Equal(t, "late", body)
	})
	t.Run("delegated", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "owner.yaml")
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("version: 2\napex: 127.0.0.1:21958\ntunnels:\n  - target: %s\n", target.URL)), 0600))
		ownerLogs, stopOwner := startIntegrationApp(t, clientcmd.Generate(), "client", "--insecure", "tunnel", "--config", path, "--server", "127.0.0.1:21881")
		waitIntegrationLog(t, ownerLogs, "Local server started")
		var hostnames []struct {
			Hostname string `json:"hostname"`
		}
		require.Equal(t, 200, localTokenRequest(t, 21881, "GET", "/ls", nil, &hostnames))
		require.Len(t, hostnames, 1)
		hostname := hostnames[0].Hostname
		require.Equal(t, 200, localTokenRequest(t, 21881, "POST", "/unpublish/"+hostname, nil, nil))
		var minted struct {
			Token string `json:"token"`
			Grant struct {
				ID string `json:"id"`
			} `json:"grant"`
		}
		require.Equal(t, 201, localTokenRequest(t, 21881, "POST", "/tokens", map[string]string{"hostname": hostname}, &minted))
		require.Contains(t, minted.Token, "tg1_")
		tokenPath := filepath.Join(t.TempDir(), "token")
		require.NoError(t, os.WriteFile(tokenPath, []byte(minted.Token), 0600))
		logs, stopServe := startIntegrationApp(t, clientcmd.Generate(), "client", "--insecure", "serve", "--apex", "127.0.0.1:21959", "--token-file", tokenPath, target.URL)
		authority := lightweightAuthority(t, logs)
		activeConnections := func() map[string]string {
			active := make(map[string]string)
			for _, entry := range logs.All() {
				fields := entry.ContextMap()
				slot := fmt.Sprint(fields["slot"])
				server, _ := fields["server"].(string)
				switch entry.Message {
				case "Tunnel connection ready":
					active[slot] = server
				case "Tunnel disconnected":
					if active[slot] == server {
						delete(active, slot)
					}
				}
			}
			return active
		}
		require.Eventually(t, func() bool {
			active := activeConnections()
			servers := make(map[string]bool)
			for _, slot := range []string{"1", "2", "3"} {
				if active[slot] == "" {
					return false
				}
				servers[active[slot]] = true
			}
			return len(active) == 3 && len(servers) == 3
		}, 45*time.Second, 20*time.Millisecond, "discover and activate three distinct servers from the apex")
		require.Equal(t, 1, logs.FilterMessage("Tunnel ready").Len())
		for _, entry := range logs.FilterMessage("Tunnel connection ready").All() {
			fields := entry.ContextMap()
			require.Equal(t, minted.Grant.ID, fields["grantId"])
			require.Equal(t, "none", fields["expiresAt"])
		}
		for _, port := range ports {
			code, body := fetchLightweight(t, authority, port)
			require.Equal(t, 200, code, body)
			require.Equal(t, "lightweight", body)
		}
		require.NoError(t, stopOwner())
		before, err := os.ReadFile(path)
		require.NoError(t, err)
		out, err := runTokenCLI(t, "list", "--config", path)
		require.NoError(t, err)
		require.Contains(t, out, minted.Grant.ID)
		require.NotContains(t, out, "tg1_")
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, sha256.Sum256(before), sha256.Sum256(after))
		out, err = runTokenCLI(t, "revoke", "--config", path, minted.Grant.ID)
		require.NoError(t, err)
		require.Contains(t, out, `"revoked": true`)
		app, _ := compileApp(clientcmd.Generate())
		app.Metadata["apexOverride"] = serverApex
		app.Writer = io.Discard
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		err = app.Run(ctx, []string{"specter", "client", "--insecure", "serve", "--apex", "127.0.0.1:21959", "--token-file", tokenPath, target.URL})
		require.ErrorContains(t, err, "unknown or revoked grant")
		require.Eventually(t, func() bool { return len(activeConnections()) == 0 }, 45*time.Second, 100*time.Millisecond, "revocation closes all token connections")
		for _, port := range ports {
			code, body := fetchLightweight(t, authority, port)
			require.Equal(t, 503, code, body)
		}
		// Cancellation may beat the client's next Open after its last connection
		// closes; otherwise that Open must fail with the revoked-grant error.
		if err := stopServe(); err != nil {
			var rpcError twirp.Error
			require.True(t, errors.As(err, &rpcError), "%v", err)
			require.Equal(t, twirp.Unauthenticated, rpcError.Code())
			require.Equal(t, "unknown or revoked grant", rpcError.Msg())
		}
	})
}
