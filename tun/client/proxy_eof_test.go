package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHTTPProxyUpstreamEOF(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
			return
		}
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	}))
	defer upstream.Close()

	u, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	f := newForwarder(zap.NewNop())
	defer f.closeAll()
	hp := f.getHTTPProxy(t.Context(), "eof", route{parsed: u})
	proxy := httptest.NewServer(hp.forwarder.Handler)
	defer proxy.Close()

	visitor := proxy.Client()
	visitor.Timeout = 5 * time.Second
	resp, err := visitor.Post(proxy.URL, "text/plain", strings.NewReader("complete request"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.Equal(t, "Forwarding target returned error: EOF", string(body))
}

func TestHTTPProxyCanceledRequest(t *testing.T) {
	f := newForwarder(zap.NewNop())
	defer f.closeAll()
	hp := f.getHTTPProxy(t.Context(), "canceled", route{parsed: &url.URL{Scheme: "http", Host: "upstream.invalid"}})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "http://app.example.com/", nil)
	rw := httptest.NewRecorder()
	rw.Code = 0 // Distinguish an unwritten response from an implicit 200.
	hp.forwarder.Handler.ServeHTTP(rw, req)
	require.Zero(t, rw.Code)
	require.Empty(t, rw.Body.String())
}
