package integrations

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	clientcmd "go.miragespace.co/specter/cmd/client"
	servercmd "go.miragespace.co/specter/cmd/server"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/waf"

	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestIntegrationWAF(t *testing.T) {
	if os.Getenv("GO_INTEGRATION_TUNNEL") == "" {
		t.Skip("skipping integration tests")
	}
	ports := []int{21970, 21971}
	for i, port := range ports {
		args := []string{"server", "--cert-dir", "../certs", "--data-dir", t.TempDir(), "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--listen-http", fmt.Sprint(21870 + i), "--apex", serverApex, "--virtual", "1", "--auth_user", "operator", "--auth_pass", "waf-test"}
		if i > 0 {
			args = append(args, "--join", fmt.Sprintf("127.0.0.1:%d", ports[0]))
		}
		logs, _ := startIntegrationApp(t, servercmd.Generate(), args...)
		waitIntegrationLog(t, logs, "gateway server started")
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/request-id" {
			w.Header().Set("X-Request-ID", "upstream-id")
			w.WriteHeader(http.StatusEarlyHints)
			_, _ = io.WriteString(w, r.Header.Get("X-Request-ID"))
			return
		}
		if r.URL.Path == "/ws" {
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			kind, body, err := conn.ReadMessage()
			if err == nil {
				_ = conn.WriteMessage(kind, body)
			}
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(target.Close)
	file := filepath.Join(t.TempDir(), "client.yaml")
	require.NoError(t, os.WriteFile(file, []byte(fmt.Sprintf("version: 2\napex: 127.0.0.1:%d\ntunnels:\n  - target: %s\n", ports[0], target.URL)), 0600))
	logs, _ := startIntegrationApp(t, clientcmd.Generate(), "client", "--insecure", "tunnel", "--config", file, "--server", "127.0.0.1:21972")
	waitIntegrationLog(t, logs, "Tunnel published")
	host := logs.FilterMessage("Tunnel published").All()[0].ContextMap()["hostname"].(string)

	newClient := func(port int, authority string, h2 bool) *http.Client {
		cfg := &tls.Config{ServerName: authority, InsecureSkipVerify: true}
		if h2 {
			cfg.NextProtos = []string{"h2"}
		} else {
			cfg.NextProtos = []string{"http/1.1"}
		}
		tp := &http.Transport{ForceAttemptHTTP2: h2, TLSClientConfig: cfg,
			DialTLSContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&tls.Dialer{Config: cfg}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
			},
		}
		t.Cleanup(tp.CloseIdleConnections)
		return &http.Client{Transport: tp, Timeout: 10 * time.Second}
	}
	type operatorSnapshot struct {
		Version string          `json:"version"`
		Config  json.RawMessage `json:"config"`
		Status  waf.Status      `json:"status"`
	}
	operatorClients := []*http.Client{newClient(ports[0], serverApex, true), newClient(ports[1], serverApex, true)}
	operatorCall := func(index int, method string, value proto.Message) operatorSnapshot {
		t.Helper()
		var data []byte
		if value != nil {
			var err error
			data, err = waf.MarshalJSON(value)
			require.NoError(t, err)
		}
		req, err := http.NewRequest(method, "https://"+serverApex+"/_internal/api/config/waf", bytes.NewReader(data))
		require.NoError(t, err)
		req.SetBasicAuth("operator", "waf-test")
		req.Header.Set("Content-Type", "application/json")
		resp, err := operatorClients[index].Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		var snapshot operatorSnapshot
		require.NoError(t, json.Unmarshal(body, &snapshot))
		return snapshot
	}
	initial := operatorCall(0, http.MethodGet, nil)
	initialConfig := new(protocol.WAFSettings)
	require.NoError(t, waf.DecodeJSON(bytes.NewReader(initial.Config), initialConfig))
	require.False(t, initialConfig.Enabled)
	settings := waf.DefaultSettings()
	settings.Enabled = true
	settings.PinnedVersion = "4.25.2" // Embedded release keeps this integration independent of upstream availability.
	saved := operatorCall(0, http.MethodPut, &protocol.SetWAFSettingsRequest{ExpectedVersion: initial.Version, Config: settings})
	for i := range ports {
		require.Eventually(t, func() bool {
			state := operatorCall(i, http.MethodGet, nil)
			return state.Status.Ready && state.Status.Enabled && state.Status.AppliedVersion == saved.Version
		}, 30*time.Second, time.Second, "WAF intent must replicate and activate")
	}
	attackPath := "/allowed?q=" + url.QueryEscape("<script>alert(1)</script>")
	request := func(c *http.Client, path string, body string) (int, []byte, string) {
		t.Helper()
		method := http.MethodGet
		if body != "" {
			method = http.MethodPost
		}
		r, err := http.NewRequest(method, "https://"+host+path, strings.NewReader(body))
		require.NoError(t, err)
		r.Header.Set("Accept", "application/json")
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.Do(r)
		require.NoError(t, err)
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, data, resp.Header.Get("X-Request-ID")
	}
	clients := []*http.Client{newClient(ports[0], host, false), newClient(ports[1], host, true)}
	h3 := &http3.Transport{TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}, Dial: func(ctx context.Context, _ string, cfg *tls.Config, qc *quic.Config) (*quic.Conn, error) {
		return quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", ports[0]), cfg, qc)
	}}
	t.Cleanup(func() { _ = h3.Close() })
	clients = append(clients, &http.Client{Transport: h3, Timeout: 10 * time.Second})
	var denial struct {
		RequestID string `json:"request_id"`
		Rules     []struct {
			ID         int  `json:"id"`
			Excludable bool `json:"excludable"`
		} `json:"rules"`
	}
	for _, c := range clients {
		code, data, requestID := request(c, attackPath, "")
		require.Equal(t, http.StatusForbidden, code, string(data))
		require.NoError(t, json.Unmarshal(data, &denial))
		require.NotEmpty(t, requestID)
		require.Equal(t, requestID, denial.RequestID)
	}
	missing := "missing-waf-test." + serverApex
	missingResponse, err := newClient(ports[0], missing, false).Get("https://" + missing + attackPath)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, missingResponse.Body)
	missingResponse.Body.Close()
	require.Equal(t, http.StatusNotFound, missingResponse.StatusCode, "missing routes must be rejected before attack inspection")

	var policyJSON json.RawMessage
	policy := new(protocol.HostWAFResponse)
	policyPath := "/waf?hostname=" + url.QueryEscape(strings.TrimSuffix(host, "."+serverApex))
	require.Equal(t, http.StatusOK, localTokenRequest(t, 21972, http.MethodGet, policyPath, nil, &policyJSON))
	require.NoError(t, waf.DecodeJSON(bytes.NewReader(policyJSON), policy))
	for _, rule := range denial.Rules {
		if rule.Excludable {
			policy.Policy.Exclusions = append(policy.Policy.Exclusions, &protocol.WAFExclusion{RuleId: uint32(rule.ID), Path: "/allowed"})
		}
	}
	require.NotEmpty(t, policy.Policy.Exclusions)
	policyRequest, err := waf.MarshalJSON(&protocol.SetHostWAFRequest{ExpectedVersion: policy.Version, Policy: policy.Policy})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, localTokenRequest(t, 21972, http.MethodPut, policyPath, json.RawMessage(policyRequest), &policyJSON))
	for _, c := range clients {
		require.Eventually(t, func() bool {
			code, _, _ := request(c, attackPath, "")
			return code == http.StatusOK
		}, 35*time.Second, time.Second, "saved hostname policy must become visible")
		code, data, _ := request(c, strings.Replace(attackPath, "/allowed", "/other", 1), "")
		require.Equal(t, http.StatusForbidden, code, string(data))
	}
	body := `{"message":"` + strings.Repeat("a", waf.DefaultBodyLimit*2) + `"}`
	code, received, _ := request(clients[1], "/upload", body)
	require.Equal(t, http.StatusOK, code, string(received))
	require.Len(t, received, len(body), "inspection must preserve upload length")
	require.Equal(t, body, string(received), "inspection must replay the complete upload")

	var earlyHints atomic.Bool
	hintContext := httptrace.WithClientTrace(t.Context(), &httptrace.ClientTrace{
		Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
			if code == http.StatusEarlyHints {
				earlyHints.Store(true)
			}
			return nil
		},
	})
	hintRequest, err := http.NewRequestWithContext(hintContext, http.MethodGet, "https://"+host+"/request-id", nil)
	require.NoError(t, err)
	hintResponse, err := clients[0].Do(hintRequest)
	require.NoError(t, err)
	receivedID, err := io.ReadAll(hintResponse.Body)
	hintResponse.Body.Close()
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, hintResponse.StatusCode)
	require.True(t, earlyHints.Load())
	require.NotEmpty(t, receivedID)
	require.Equal(t, []string{string(receivedID)}, hintResponse.Header.Values("X-Request-ID"), "final response must preserve the gateway ID after Early Hints")
	ws, _, err := (&websocket.Dialer{TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}, NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", ports[0]))
	}}).DialContext(t.Context(), "wss://"+host+"/ws", nil)
	require.NoError(t, err)
	defer ws.Close()
	require.NoError(t, ws.SetReadDeadline(time.Now().Add(5*time.Second)))
	require.NoError(t, ws.WriteMessage(websocket.TextMessage, []byte("hello")))
	_, received, err = ws.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "hello", string(received))
}
