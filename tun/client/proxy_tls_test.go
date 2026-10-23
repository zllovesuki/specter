package client

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestHTTPProxyHTTPSWithPort(t *testing.T) {
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Host)
	}))
	defer backend.Close()
	roots := x509.NewCertPool()
	roots.AddCert(backend.Certificate())

	u, err := url.Parse(backend.URL)
	require.NoError(t, err)
	f := newForwarder(zaptest.NewLogger(t))
	proxy := f.newHTTPProxy(testHostname, Tunnel{parsed: u})
	defer proxy.close()
	proxy.transport.TLSClientConfig.RootCAs = roots
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/", nil)
	result := httptest.NewRecorder()
	proxy.forwarder.Handler.ServeHTTP(result, req)

	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Equal(t, u.Host, result.Body.String())
}
