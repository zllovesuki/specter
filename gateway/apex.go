package gateway

import (
	_ "embed"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	"kon.nect.sh/specter/spec/protocol"
	"kon.nect.sh/specter/util"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const bodyLimit = 1 << 10 // 1kb

//go:embed index.html
var index string

//go:embed quic.png
var quicPng []byte

var view = template.Must(template.New("index").Parse(index))

type apexServer struct {
	handlers      InternalHandlers
	limiter       func(http.Handler) http.Handler
	internalProxy func(http.Handler) http.Handler
	pkiServer     protocol.PKIService
	authUser      string
	authPass      string
}

func (a *apexServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	var root string
	if r.ProtoAtLeast(2, 0) {
		root = r.Host
	} else {
		root = r.TLS.ServerName
	}
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	view.Execute(w, struct {
		RootDomain string
	}{
		RootDomain: root,
	})
}

func (a *apexServer) handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(quicPng)))
	w.Write(quicPng)
}

func (a *apexServer) Mount(r *chi.Mux) {
	r.Use(a.limiter)
	r.Use(middleware.Recoverer)
	r.Use(middleware.NoCache)
	r.Use(middleware.Compress(5, "text/*"))
	r.Get("/", a.handleRoot)
	r.Get("/quic.png", a.handleLogo)
	if a.pkiServer != nil {
		pkiServer := protocol.NewPKIServiceServer(a.pkiServer)
		r.With(util.LimitBody(bodyLimit)).Mount(pkiServer.PathPrefix(), pkiServer)
	}
	if a.authUser == "" || a.authPass == "" {
		return
	}
	r.Route("/_internal", func(r chi.Router) {
		r.Use(middleware.BasicAuth("internal", map[string]string{
			a.authUser: a.authPass,
		}))
		r.Use(a.internalProxy)
		if a.handlers.ChordStats != nil {
			r.Mount("/chord", a.handlers.ChordStats)
		}
		if a.handlers.ClientsQuery != nil {
			r.Mount("/clients", a.handlers.ClientsQuery)
		}
		r.Handle("/migrator", a.handlers.MigrationHandler)
		r.Mount("/debug", middleware.Profiler())

		// catch-all helper
		routes := make([]string, 0)
		chi.Walk(r, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
			if method == http.MethodGet && !strings.Contains(route, "*") {
				routes = append(routes, fmt.Sprintf("%s %s", method, route))
			}
			return nil
		})
		routesStr := []byte(strings.Join(routes, "\n"))
		r.HandleFunc("/*", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write(routesStr)
		})
	})
}
