package gateway

import (
	_ "embed"
	"net/http"
	"strconv"
	"text/template"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed index.html
var index string

//go:embed quic.png
var quicPng []byte

var view = template.Must(template.New("index").Parse(index))

type apexServer struct {
	statsHandler http.HandlerFunc
	rootDomain   string
}

func (a *apexServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	view.Execute(w, struct {
		RootDomain string
	}{
		RootDomain: a.rootDomain,
	})
}

func (a *apexServer) handleLogo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(quicPng)))
	w.Write(quicPng)
}

func (a *apexServer) Mount(r *chi.Mux) {
	r.Get("/", a.handleRoot)
	r.Get("/quic.png", a.handleLogo)
	r.Mount("/debug", middleware.Profiler())
}

func (a *apexServer) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", a.handleRoot)
	r.Route("/_internal", func(r chi.Router) {
		r.Use(middleware.BasicAuth("internal", map[string]string{
			"test": "test",
		}))
		r.Get("/stats", a.statsHandler)
		r.Mount("/debug", middleware.Profiler())
	})
	return r
}
