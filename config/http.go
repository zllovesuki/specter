package config

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"
)

type HTTPErrorResponse struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteSaveError handles the shared store's save outcomes for document editors.
// It returns false on success so the caller can render its document response.
func WriteSaveError(w http.ResponseWriter, name string, logger *zap.Logger, err error) bool {
	if err == nil {
		return false
	}
	status := http.StatusInternalServerError
	response := HTTPErrorResponse{Code: "save_failed", Message: "Could not save the configuration on this server."}
	switch {
	case errors.Is(err, ErrConflict), errors.Is(err, ErrSchemaRegression):
		status = http.StatusConflict
		response = HTTPErrorResponse{Code: "version_conflict", Message: "Settings changed. Load the latest settings before saving again."}
	case errors.Is(err, ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
		response = HTTPErrorResponse{Code: "validation_failed", Message: "The shared configuration is too large."}
	default:
		logger.Error("Could not save shared configuration", zap.String("config", name), zap.Error(err))
		if errors.Is(err, ErrDurabilityUncertain) {
			status = http.StatusServiceUnavailable
			response = HTTPErrorResponse{Code: "outcome_unknown", Message: "The save result is uncertain. Retry the save or refresh to review the current configuration."}
		}
	}
	WriteJSON(w, status, response)
	return true
}
