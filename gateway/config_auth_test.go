package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestConfigurationUsesOperatorAuthenticationAndBrowserProtection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		auth   bool
		origin string
		site   string
		want   int
	}{
		{name: "unauthenticated", want: http.StatusUnauthorized},
		{name: "same origin", auth: true, origin: "https://operator.example.com", site: "same-origin", want: http.StatusNoContent},
		{name: "command line", auth: true, want: http.StatusNoContent},
		{name: "cross origin", auth: true, origin: "https://evil.example.net", site: "cross-site", want: http.StatusForbidden},
		{name: "origin fallback", auth: true, origin: "https://evil.example.net", want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called, proxied := false, false
			pass := func(next http.Handler) http.Handler { return next }
			apex := &apexServer{
				authUser: "operator", authPass: "test-password", limiter: pass,
				internalProxy: func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						proxied = true
						next.ServeHTTP(w, r)
					})
				},
				handlers: InternalHandlers{Configuration: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					called = true
					w.WriteHeader(http.StatusNoContent)
				})},
			}
			router := chi.NewRouter()
			apex.Mount(router)
			request := httptest.NewRequest(http.MethodPut, "https://operator.example.com/_internal/api/config/dns", nil)
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Sec-Fetch-Site", tc.site)
			if tc.auth {
				request.SetBasicAuth("operator", "test-password")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.want, response.Code)
			require.Equal(t, tc.want == http.StatusNoContent, called)
			require.Equal(t, called, proxied, "browser check must precede internal proxying")
		})
	}
}
