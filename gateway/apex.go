package gateway

import (
	_ "embed"
	"net/http"
	"strconv"
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
		r.Use(middleware.URLFormat)
		r.Get("/stats", a.handlers.StatsHandler)
		r.Get("/graph", a.handlers.RingGraphHandler)
		r.Handle("/migrator", a.handlers.MigrationHandler)
		r.Mount("/debug", middleware.Profiler())
	})
}
