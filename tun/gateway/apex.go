package gateway

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"

	"kon.nect.sh/specter/spec/chord"
	"kon.nect.sh/specter/spec/gateway"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed index.html
var index string

var view = template.Must(template.New("index").Parse(index))

type apexServer struct {
	chord      chord.VNode
	rootDomain string
	clientPort int
}

func (a *apexServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	view.Execute(w, struct {
		RootDomain string
	}{
		RootDomain: a.rootDomain,
	})
}

func (a *apexServer) handleLookup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(gateway.LookupResponse{
		Address: a.rootDomain,
		Port:    a.clientPort,
	})
}

func (a *apexServer) handleKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := a.chord.LocalKeys(0, 0)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "text/plain")
	fmt.Fprintf(w, "keys on node %d:\n", a.chord.ID())
	for _, key := range keys {
		fmt.Fprintf(w, "  %15d - %s\n", chord.Hash(key), key)
	}
}

func (a *apexServer) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", a.handleRoot)
	r.Get("/lookup", a.handleLookup)
	r.Route("/_internal", func(r chi.Router) {
		r.Use(middleware.BasicAuth("internal endpoints", map[string]string{
			"test": "test",
		}))
		r.Get("/keys", a.handleKeys)
		r.Mount("/debug", middleware.Profiler())
	})
	return r
}
