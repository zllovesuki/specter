package acme

import (
	"net/http"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/go-chi/chi/v5"
)

func AcmeManagerHandler(m *Manager) http.Handler {
	router := chi.NewRouter()

	router.Post("/clean", func(w http.ResponseWriter, r *http.Request) {
		certmagic.CleanStorage(r.Context(), m.chordStorage, certmagic.CleanStorageOptions{
			OCSPStaples:            true,
			ExpiredCerts:           true,
			ExpiredCertGracePeriod: time.Hour * 24 * 30,
		})
		w.WriteHeader(http.StatusNoContent)
	})

	return router
}
