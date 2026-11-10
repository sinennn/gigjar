package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"gigjar/internal/ai"
	"gigjar/internal/database"
	"gigjar/internal/googleauth"
	"gigjar/internal/identity"
	"gigjar/internal/jars"
	"gigjar/internal/paylinks"
	"gigjar/internal/tron"
)

type RouterConfig struct {
	Logger         *slog.Logger
	AllowedOrigins []string
	RequestTimeout time.Duration
	Readiness      ReadinessProbe
	DB             database.Querier
	Identity       *identity.Service
	Jars           *jars.Service
	PayLinks       *paylinks.Service
	Network        tron.Network
	PublicWebURL   string
	SecureCookies  bool
	Google         *googleauth.Client
	AI             *ai.Service
}

type api struct {
	logger        *slog.Logger
	db            database.Querier
	identity      *identity.Service
	jars          *jars.Service
	paylinks      *paylinks.Service
	network       tron.Network
	publicWebURL  string
	secureCookies bool
	loginAttempts *attemptLimiter
	google        *googleauth.Client
	ai            *ai.Service
	aiPerUser     *attemptLimiter
	aiGlobal      *attemptLimiter
}

func NewRouter(cfg RouterConfig) (http.Handler, error) {
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

	crossOrigin := http.NewCrossOriginProtection()
	for _, origin := range cfg.AllowedOrigins {
		if err := crossOrigin.AddTrustedOrigin(origin); err != nil {
			return nil, fmt.Errorf("trusted origin %q: %w", origin, err)
		}
	}
	crossOrigin.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, cfg.Logger, http.StatusForbidden, "cross_origin_rejected", "cross-origin request rejected")
	}))

	health := healthHandlers{logger: cfg.Logger, readiness: cfg.Readiness}
	router.Get("/healthz", health.live)
	router.Get("/readyz", health.ready)

	a := &api{
		logger:        cfg.Logger,
		db:            cfg.DB,
		identity:      cfg.Identity,
		jars:          cfg.Jars,
		paylinks:      cfg.PayLinks,
		network:       cfg.Network,
		publicWebURL:  cfg.PublicWebURL,
		secureCookies: cfg.SecureCookies,
		loginAttempts: newAttemptLimiter(10, time.Minute),
		google:        cfg.Google,
		ai:            cfg.AI,
		aiPerUser:     newAttemptLimiter(30, time.Hour),
		aiGlobal:      newAttemptLimiter(25, time.Minute),
	}

	router.Route("/v1", func(v1 chi.Router) {
		v1.Use(crossOrigin.Handler)

		v1.Post("/auth/register", a.register)
		v1.Post("/auth/login", a.login)
		v1.Post("/auth/logout", a.logout)
		v1.Get("/auth/providers", a.getProviders)
		v1.Get("/auth/google/start", a.startGoogleSignIn)
		v1.Get("/auth/google/callback", a.finishGoogleSignIn)
		v1.Get("/public/pay/{reference}", a.getPublicPayment)
		if cfg.Network.IsSimulated() {
			v1.Post("/simulator/transfers", a.sendSimulatedTransfer)
		}

		v1.Group(func(private chi.Router) {
			private.Use(a.requireSession)
			private.Get("/me", a.getMe)
			private.Patch("/me", a.updateMe)
			private.Get("/jar", a.getJar)
			private.Get("/wallet/cleva", a.getDestination)
			private.Put("/wallet/cleva", a.setDestination)
			private.Get("/payment-links", a.listPaymentLinks)
			private.Post("/payment-links", a.createPaymentLink)
			private.Get("/payment-links/{id}", a.getPaymentLink)
			private.Post("/payment-links/{id}/close", a.closePaymentLink)
			private.Get("/transactions", a.listTransactions)
			private.Get("/incoming-transfers", a.listIncomingTransfers)
			private.Get("/settlements", a.listSettlements)
			private.Post("/ai/link-draft", a.draftPaymentLink)
			private.Get("/ai/insights", a.getInsights)
			private.Post("/ai/assistant", a.askAssistant)
		})
	})

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, cfg.Logger, http.StatusNotFound, "not_found", "route not found")
	})
	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, cfg.Logger, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed on this route")
	})

	return router, nil
}
