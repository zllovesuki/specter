package waf

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/spec/protocol"
	"go.miragespace.co/specter/util"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type settingsResponse struct {
	Version    string          `json:"version"`
	Config     json.RawMessage `json:"config"`
	Server     settingsServer  `json:"server"`
	ObservedAt time.Time       `json:"observed_at"`
	Status     Status          `json:"status"`
	Error      string          `json:"error,omitempty"`
}

type settingsServer struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

// SettingsHandler must be mounted behind the existing operator authentication.
func SettingsHandler(store *config.Store, identity *protocol.Node, status func() Status, wake func(), logger *zap.Logger) http.Handler {
	respond := func(w http.ResponseWriter, record config.Record, found bool) {
		s := DefaultSettings()
		response := settingsResponse{Version: record.Token(),
			Server:     settingsServer{ID: strconv.FormatUint(identity.GetId(), 10), Address: identity.GetAddress()},
			ObservedAt: time.Now().UTC(), Status: status(),
		}
		if found {
			decoded, err := DecodeSettings(record)
			if err != nil {
				s = nil
				response.Error = err.Error()
			} else {
				s = decoded
			}
		}
		if s != nil {
			data, err := MarshalJSON(s)
			if err != nil {
				config.WriteJSON(w, http.StatusInternalServerError, config.HTTPErrorResponse{Code: "read_failed", Message: "Could not encode settings."})
				return
			}
			response.Config = data
		}
		config.WriteJSON(w, http.StatusOK, response)
	}
	failure := func(w http.ResponseWriter, code int, name, message string) {
		config.WriteJSON(w, code, config.HTTPErrorResponse{Code: name, Message: message})
	}
	router := chi.NewRouter()
	router.Use(middleware.SetHeader("Cache-Control", "no-store"))
	router.Get("/waf", func(w http.ResponseWriter, r *http.Request) {
		record, found, err := store.Get(ConfigName)
		if err != nil {
			logger.Error("Could not read WAF settings", zap.Error(err))
			failure(w, http.StatusServiceUnavailable, "not_ready", "Could not read settings on this server. Try refreshing shortly.")
			return
		}
		if after := r.URL.Query().Get("after"); after != "" {
			candidate := record
			if !found {
				candidate.Name = ConfigName
			}
			comparison, err := candidate.CompareVersion(after)
			if err != nil {
				failure(w, http.StatusBadRequest, "invalid_version", "The configuration version is invalid.")
				return
			}
			if !found || comparison < 0 {
				failure(w, http.StatusServiceUnavailable, "not_ready", "This server has not caught up with the settings you last viewed. Try again shortly.")
				return
			}
		}
		respond(w, record, found)
	})
	router.With(util.LimitBody(8<<10)).Put("/waf", func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			failure(w, http.StatusUnsupportedMediaType, "validation_failed", "Send settings as application/json.")
			return
		}
		input := new(protocol.SetWAFSettingsRequest)
		if err := DecodeJSON(r.Body, input); err != nil {
			failure(w, http.StatusBadRequest, "validation_failed", "Invalid settings JSON.")
			return
		}
		current, found, err := store.Get(ConfigName)
		if err != nil {
			failure(w, http.StatusServiceUnavailable, "not_ready", "Could not read settings on this server.")
			return
		}
		if current.Token() != input.ExpectedVersion {
			failure(w, http.StatusConflict, "version_conflict", "Settings changed. Load the latest settings before saving again.")
			return
		}
		if found {
			if _, err := DecodeSettings(current); err != nil {
				failure(w, http.StatusConflict, "unsupported_configuration", "Use a compatible Specter version to edit the stored WAF settings.")
				return
			}
		}
		s, err := ValidateSettings(input.Config)
		if err != nil {
			failure(w, http.StatusBadRequest, "validation_failed", err.Error())
			return
		}
		payload, err := MarshalJSON(s)
		if err != nil {
			failure(w, http.StatusInternalServerError, "save_failed", "Could not encode settings.")
			return
		}
		record, err := store.Put(ConfigName, ConfigSchema, payload, input.ExpectedVersion)
		if wake != nil && (err == nil || errors.Is(err, config.ErrDurabilityUncertain)) {
			wake()
		}
		if !config.WriteSaveError(w, ConfigName, logger, err) {
			respond(w, record, true)
		}
	})
	return router
}
