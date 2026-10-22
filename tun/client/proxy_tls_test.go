package client

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestHTTPProxyHTTPSWithPort(t *testing.T) {
	// Isolate the process-wide trust roots from the rest of the test suite.
	if os.Getenv("SPECTER_TEST_HTTPS_PROXY") == "" {
		exe, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestHTTPProxyHTTPSWithPort$")
		cmd.Env = append(os.Environ(), "SPECTER_TEST_HTTPS_PROXY=1", "GODEBUG="+os.Getenv("GODEBUG")+",x509usefallbackroots=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}

	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Host)
	}))
	defer backend.Close()
	roots := x509.NewCertPool()
	roots.AddCert(backend.Certificate())
	x509.SetFallbackRoots(roots)

	u, err := url.Parse(backend.URL)
	require.NoError(t, err)
	f := newForwarder(zaptest.NewLogger(t))
	proxy := f.newHTTPProxy(testHostname, Tunnel{parsed: u})
	defer proxy.close()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/", nil)
	result := httptest.NewRecorder()
	proxy.forwarder.Handler.ServeHTTP(result, req)

	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	require.Equal(t, u.Host, result.Body.String())
}
