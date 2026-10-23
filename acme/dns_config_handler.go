package acme

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

type dnsConfigHandler struct {
	store    *config.Store
	identity *protocol.Node
	zone     string
	logger   *zap.Logger
	wake     func()
}

type configServerIdentity struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type dnsConfigResponse struct {
	Version    string               `json:"version"`
	Config     *DNSConfig           `json:"config"`
	Server     configServerIdentity `json:"server"`
	ObservedAt time.Time            `json:"observed_at"`
	Error      string               `json:"error,omitempty"`
}

type configError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// DNSConfigHandler serves the DNS document editor API. Mount it behind operator authentication.
func DNSConfigHandler(store *config.Store, identity *protocol.Node, zone string, logger *zap.Logger, wake func()) http.Handler {
	h := &dnsConfigHandler{store: store, identity: identity, zone: zone, logger: logger, wake: wake}
	router := chi.NewRouter()
	router.Use(middleware.SetHeader("Cache-Control", "no-store"))
	router.Get("/dns", h.get)
	router.With(util.LimitBody(config.MaxValueBytes+4096)).Put("/dns", h.put)
	return router
}

func (h *dnsConfigHandler) response(record config.Record, found bool) dnsConfigResponse {
	response := dnsConfigResponse{
		Server:     configServerIdentity{ID: strconv.FormatUint(h.identity.GetId(), 10), Address: h.identity.GetAddress()},
		ObservedAt: time.Now().UTC(),
	}
	if found {
		response.Version = record.Token()
		dnsConfig, err := DecodeDNSConfig(record, h.zone)
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Config = &dnsConfig
		}
	}
	return response
}

func (h *dnsConfigHandler) get(w http.ResponseWriter, r *http.Request) {
	record, found, err := h.store.Get(DNSConfigName)
	if err != nil {
		h.logger.Error("Could not read shared DNS configuration", zap.Error(err))
		writeConfigJSON(w, http.StatusServiceUnavailable, configError{Code: "not_ready", Message: "Could not read the configuration on this server. Try refreshing again shortly."})
		return
	}
	// Keep this editor's reads from moving backward without waiting for peers.
	if after := r.URL.Query().Get("after"); after != "" {
		if !found {
			// Validate the token's scope even before this replica has a document.
			record.Name = DNSConfigName
		}
		comparison, err := record.CompareVersion(after)
		if err != nil {
			writeConfigJSON(w, http.StatusBadRequest, configError{Code: "invalid_version", Message: "The configuration version is invalid."})
			return
		}
		if !found || comparison < 0 {
			writeConfigJSON(w, http.StatusServiceUnavailable, configError{Code: "not_ready", Message: "This server has not caught up with the configuration you last viewed. Try again shortly."})
			return
		}
	}
	writeConfigJSON(w, http.StatusOK, h.response(record, found))
}

func (h *dnsConfigHandler) put(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeConfigJSON(w, http.StatusUnsupportedMediaType, configError{Code: "validation_failed", Message: "Send configuration as application/json."})
		return
	}
	var input struct {
		ExpectedVersion string    `json:"expected_version"`
		Config          DNSConfig `json:"config"`
	}
	if err := util.DecodeJSON(r.Body, &input); err != nil {
		writeConfigJSON(w, http.StatusBadRequest, configError{Code: "validation_failed", Message: "Invalid configuration JSON."})
		return
	}
	current, found, err := h.store.Get(DNSConfigName)
	if err != nil {
		h.logger.Error("Could not read shared DNS configuration", zap.Error(err))
		writeConfigJSON(w, http.StatusServiceUnavailable, configError{Code: "not_ready", Message: "Could not read the configuration on this server. Try again shortly."})
		return
	}
	if current.Token() != input.ExpectedVersion {
		h.conflict(w)
		return
	}
	zone := h.zone
	if found {
		previous, err := DecodeDNSConfig(current, h.zone)
		if err != nil {
			writeConfigJSON(w, http.StatusConflict, configError{Code: "unsupported_configuration", Message: "This server cannot edit the stored DNS configuration. Use a compatible Specter version."})
			return
		}
		// Changing the hosted zone also changes deployment and certificate
		// settings; the DNS records editor does not perform that migration.
		zone = previous.Zone
	}
	dnsConfig, err := ValidateDNSConfig(input.Config, zone)
	if err != nil {
		response := configError{Code: "validation_failed", Message: err.Error()}
		var field *DNSConfigError
		if errors.As(err, &field) {
			response.Fields = map[string]string{field.Field: field.Message}
		}
		writeConfigJSON(w, http.StatusBadRequest, response)
		return
	}
	payload, err := json.Marshal(dnsConfig)
	if err != nil {
		writeConfigJSON(w, http.StatusInternalServerError, configError{Code: "save_failed", Message: "Could not encode the configuration."})
		return
	}
	record, err := h.store.Put(DNSConfigName, DNSConfigSchema, payload, input.ExpectedVersion)
	if h.wake != nil && (err == nil || errors.Is(err, config.ErrDurabilityUncertain)) {
		h.wake()
	}
	switch {
	case errors.Is(err, config.ErrConflict), errors.Is(err, config.ErrSchemaRegression):
		h.conflict(w)
	case errors.Is(err, config.ErrDurabilityUncertain):
		h.logger.Error("Shared DNS configuration durability is uncertain", zap.Error(err))
		writeConfigJSON(w, http.StatusServiceUnavailable, configError{Code: "outcome_unknown", Message: "The save result is uncertain. Retry the save or refresh to review the current configuration."})
	case errors.Is(err, config.ErrTooLarge):
		writeConfigJSON(w, http.StatusRequestEntityTooLarge, configError{Code: "validation_failed", Message: "The shared configuration is too large."})
	case err != nil:
		h.logger.Error("Could not save shared DNS configuration", zap.Error(err))
		writeConfigJSON(w, http.StatusInternalServerError, configError{Code: "save_failed", Message: "Could not save the configuration on this server."})
	default:
		writeConfigJSON(w, http.StatusOK, h.response(record, true))
	}
}

func (h *dnsConfigHandler) conflict(w http.ResponseWriter) {
	writeConfigJSON(w, http.StatusConflict, configError{Code: "version_conflict", Message: "The stored configuration changed. Load the latest settings before saving again."})
}

func writeConfigJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
