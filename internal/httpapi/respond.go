package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.Error("write response body", "error", err)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, code, message string) {
	writeJSON(w, logger, status, ErrorBody{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		RequestID: middleware.GetReqID(r.Context()),
	}})
}
