package gateway

import (
	"fmt"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// taken from certmagic/certmagic.go

func hostOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport // OK; probably had no port to begin with
	}
	return host
}

func (g *Gateway) httpRedirect(w http.ResponseWriter, r *http.Request) {
	toURL := "https://"

	// since we redirect to the standard HTTPS port, we
	// do not need to include it in the redirect URL
	requestHost := hostOnly(r.Host)

	toURL += requestHost
	if g.GatewayPort != 443 {
		toURL = fmt.Sprintf("%s:%d", toURL, g.GatewayPort)
	}
	toURL += r.URL.RequestURI()

	// get rid of this disgusting unencrypted HTTP connection 🤢
	w.Header().Set("Connection", "close")

	http.Redirect(w, r, toURL, http.StatusMovedPermanently)
}

func (g *Gateway) httpRouter() http.Handler {
	r := chi.NewRouter()

	r.Connect("/", g.httpConnect)
	r.Handle("/*", http.HandlerFunc(g.httpRedirect))

	return r
}
