package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type RouterConfig struct {
	Logger         *slog.Logger
	AllowedOrigins []string
	RequestTimeout time.Duration
	Readiness      ReadinessProbe
}

func NewRouter(cfg RouterConfig) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(requestLogger(cfg.Logger))
	router.Use(recoverPanics(cfg.Logger))
	router.Use(securityHeaders)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	router.Use(middleware.Timeout(cfg.RequestTimeout))

	health := healthHandlers{logger: cfg.Logger, readiness: cfg.Readiness}
	router.Get("/healthz", health.live)
	router.Get("/readyz", health.ready)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, cfg.Logger, http.StatusNotFound, "not_found", "route not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, cfg.Logger, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed on this route")
	})

	return router
}
