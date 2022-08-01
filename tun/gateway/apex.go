package gateway

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"text/template"

	"kon.nect.sh/specter/spec/gateway"
)

//go:embed index.html
var index string

var view = template.Must(template.New("index").Parse(index))

type apexServer struct {
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
