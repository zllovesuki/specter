package gateway

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (g *Gateway) mountCgiHandler(r *chi.Mux) {
	r.Route("/specter-cgi", func(router chi.Router) {
		router.Use(func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				g.appendHeaders(r.ProtoAtLeast(3, 0))(w.Header())
				h.ServeHTTP(w, r)
			})
		})

		router.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "specter.identity=%s\n", g.TunnelServer.Identity().GetAddress())
			fmt.Fprintf(w, "specter.peer=%s\n", r.RemoteAddr)
			fmt.Fprintf(w, "tls.cipher=%s\n", tls.CipherSuiteName(r.TLS.CipherSuite))
			fmt.Fprintf(w, "tls.sni=%s\n", r.TLS.ServerName)
			fmt.Fprintf(w, "http.host=%s\n", r.Host)
			fmt.Fprintf(w, "http.proto=%s\n", r.Proto)
		})

		router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
