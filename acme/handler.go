package acme

import (
	"encoding/json"
	"net/http"

	"kon.nect.sh/specter/spec/protocol"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func ManagerHandler(m *Manager) http.Handler {
	router := chi.NewRouter()

	router.Get("/list/*", func(w http.ResponseWriter, r *http.Request) {
		prefix := chi.URLParam(r, "*")
		m.Logger.Debug("list request", zap.String("prefix", prefix))
		resp, err := m.KV.ListKeys(r.Context(), []byte(prefix), protocol.ListKeysRequest_SIMPLE)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(resp)
	})

	return router
}
