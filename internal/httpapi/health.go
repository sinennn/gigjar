package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

type ReadinessProbe func(ctx context.Context) error

type healthHandlers struct {
	logger    *slog.Logger
	readiness ReadinessProbe
}

type healthResponse struct {
	Status string `json:"status"`
}

func (h healthHandlers) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, h.logger, http.StatusOK, healthResponse{Status: "ok"})
}

func (h healthHandlers) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.readiness(ctx); err != nil {
		h.logger.Warn("readiness check failed", "error", err)
		writeError(w, r, h.logger, http.StatusServiceUnavailable, "not_ready", "service dependencies are not ready")
		return
	}
	writeJSON(w, h.logger, http.StatusOK, healthResponse{Status: "ready"})
}
