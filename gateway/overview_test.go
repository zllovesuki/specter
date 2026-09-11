package gateway

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"go.miragespace.co/specter/ui"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestOperatorOverviewUsesInternalAuthentication(t *testing.T) {
	page := httptest.NewRecorder()
	ui.OperatorPage().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	asset := regexp.MustCompile(`src="([^"]+)"`).FindStringSubmatch(page.Body.String())
	require.Len(t, asset, 2)
	a := &apexServer{
		authUser:      testUser,
		authPass:      testPass,
		limiter:       func(h http.Handler) http.Handler { return h },
		internalProxy: func(h http.Handler) http.Handler { return h },
		handlers: InternalHandlers{
			TunnelServer: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("client data"))
			}),
			Overview: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("local overview"))
			}),
		},
	}
	router := chi.NewRouter()
	a.Mount(router)

	for _, testCase := range []struct {
		path    string
		content string
	}{
		{"/_internal", "Specter · Operator"},
		{"/_internal/", "Specter · Operator"},
		{"/_internal/overview.json", "local overview"},
		{asset[1], ""},
		{"/_internal/tun/", "Specter · Operator"},
		{"/_internal/tun/123/client", "Specter · Operator"},
		{"/_internal/api/tun/", "client data"},
	} {
		t.Run(testCase.path, func(t *testing.T) {
			for _, authenticated := range []bool{false, true} {

				r := httptest.NewRequest(http.MethodGet, testCase.path, nil)
				if authenticated {
					r.SetBasicAuth(testUser, testPass)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if authenticated {
					require.Equal(t, http.StatusOK, w.Code)
					require.Contains(t, w.Body.String(), testCase.content)
				} else {
					require.Equal(t, http.StatusUnauthorized, w.Code)
					if testCase.content != "" {
						require.NotContains(t, w.Body.String(), testCase.content)
					}
				}
			}
		})
	}
}
