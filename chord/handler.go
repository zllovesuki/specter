package chord

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func ChordStatsHandler(rootNode *LocalNode, virtualNodes []*LocalNode) http.Handler {
	router := chi.NewRouter()

	router.Use(middleware.URLFormat)
	router.Get("/stats", statsHandler(virtualNodes))
	router.Get("/graph", ringGraphHandler(rootNode))

	return router
}
