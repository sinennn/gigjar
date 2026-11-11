package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
	"github.com/jackc/pgx/v5/pgxpool"
	"gigjar/internal/ai"
	"gigjar/internal/chain"
	"gigjar/internal/chain/simulated"
	"gigjar/internal/chain/trongrid"
	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/energy"
	"gigjar/internal/energy/tronsave"
	"gigjar/internal/googleauth"
	"gigjar/internal/httpapi"
	"gigjar/internal/identity"
	"gigjar/internal/jars"
	"gigjar/internal/mail"
	"gigjar/internal/paylinks"
	"gigjar/internal/settlement"
	"gigjar/internal/tron"
	"gigjar/internal/wallet"
	"gigjar/internal/wallet/simwallet"
	"gigjar/internal/wallet/tronwallet"
	"gigjar/internal/watcher"
)

func NewAPIHandler(cfg config.API, pool *pgxpool.Pool, logger *slog.Logger) (http.Handler, error) {
	expectedSchema, err := database.LatestEmbeddedVersion()
	if err != nil {
		return nil, err
	}
	identityService, err := identity.NewService(pool, identity.Config{
		PasswordParams: identity.DefaultPasswordParams,
		SessionTTL:     cfg.SessionTTL,
		Network:        cfg.Network,
	})
	if err != nil {
		return nil, err
	}
	var google *googleauth.Client
	if cfg.GoogleEnabled() {
		google, err = googleauth.New(googleauth.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.GoogleRedirectURL,
		})
		if err != nil {
			return nil, err
		}
	}
	assistant := ai.NewService(ai.NewGroq(ai.GroqConfig{APIKey: cfg.GroqAPIKey, Model: cfg.GroqModel}),
		pool, cfg.Network, cfg.SettlementThreshold, cfg.SettlementFee)
	var mailer mail.Sender = mail.NewLogSender(logger, !cfg.Environment.IsProduction())
	if cfg.SMTPEnabled() {
		smtpSender, err := mail.NewSMTP(mail.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.MailFrom})
		if err != nil {
			return nil, err
		}
		mailer = smtpSender
	}
	logger.Info("optional features", "google_sign_in", google != nil, "ai", assistant.Enabled(), "ai_model", cfg.GroqModel, "email", mailer.Configured())
	return httpapi.NewRouter(httpapi.RouterConfig{
		Logger:         logger,
		AllowedOrigins: cfg.CORSAllowedOrigins,
		RequestTimeout: cfg.RequestTimeout,
		Readiness:      database.SchemaReadiness(pool, expectedSchema),
		DB:             pool,
		Identity:       identityService,
		Jars:           jars.NewService(pool, cfg.SettlementThreshold, cfg.SettlementFee),
		PayLinks:       paylinks.NewService(pool),
		Network:        cfg.Network,
		PublicWebURL:   cfg.PublicWebURL,
		SecureCookies:  cfg.SecureCookies,
		Google:         google,
		AI:             assistant,
		Mailer:         mailer,
	})
}

func NewWatcher(cfg config.Watcher, pool *pgxpool.Pool, logger *slog.Logger) (*watcher.Watcher, error) {
	var provider chain.Provider
	if cfg.Network.IsSimulated() {
		provider = simulated.NewProvider(pool)
	} else {
		grid, err := trongrid.New(trongrid.Config{BaseURL: cfg.TronGridURL, APIKey: cfg.TronGridAPIKey, Network: cfg.Network})
		if err != nil {
			return nil, err
		}
		provider = grid
	}
	watcherConfig := watcher.DefaultConfig()
	watcherConfig.Tick = cfg.Tick
	watcherConfig.BatchSize = cfg.BatchSize
	watcherConfig.ActivePollInterval = cfg.ActivePollInterval
	watcherConfig.IdlePollInterval = cfg.IdlePollInterval
	watcherConfig.CursorOverlap = cfg.CursorOverlap
	watcherConfig.DropAfter = cfg.DropAfter
	watcherConfig.ReconcileInterval = cfg.ReconcileInterval
	return watcher.New(pool, provider, logger, watcherConfig), nil
}

func NewSweeper(ctx context.Context, cfg config.Sweeper, pool *pgxpool.Pool, logger *slog.Logger) (*settlement.Sweeper, error) {
	w, c, err := sweeperProviders(ctx, cfg, pool, logger)
	if err != nil {
		return nil, err
	}
	sweeperConfig := settlement.DefaultConfig()
	sweeperConfig.Threshold = cfg.SettlementThreshold
	sweeperConfig.Fee = cfg.SettlementFee
	sweeperConfig.Tick = cfg.Tick
	sweeperConfig.BatchSize = cfg.BatchSize
	return settlement.New(pool, w, c, logger, sweeperConfig)
}

func sweeperProviders(ctx context.Context, cfg config.Sweeper, pool *pgxpool.Pool, logger *slog.Logger) (wallet.Provider, chain.Provider, error) {
	if cfg.Network.IsSimulated() {
		return simwallet.New(pool, cfg.SimulatedConfirmDelay), simulated.NewProvider(pool), nil
	}
	grid, err := trongrid.New(trongrid.Config{BaseURL: cfg.TronGridURL, APIKey: cfg.TronGridAPIKey, Network: cfg.Network})
	if err != nil {
		return nil, nil, err
	}
	signer := tronwallet.NewSignerClient(cfg.SignerURL, cfg.SignerToken)
	treasury, err := waitForSigner(ctx, signer, logger)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("signer reachable", "treasury", treasury)

	var renter energy.Renter
	if cfg.EnergyProvider == "tronsave" {
		baseURL, paymentAddress, ok := tronsave.Defaults(cfg.Network)
		if !ok {
			return nil, nil, fmt.Errorf("tronsave does not support %s", cfg.Network)
		}
		if cfg.TronSaveURL != "" {
			baseURL = cfg.TronSaveURL
		}
		renter, err = tronsave.New(tronsave.Config{
			BaseURL:         baseURL,
			PaymentAddress:  paymentAddress,
			DurationSeconds: cfg.EnergyRentalSeconds,
			MaxUnitPriceSun: cfg.MaxEnergyPriceSun,
			Pay: func(ctx context.Context, to tron.Address, amountSun int64) ([]byte, string, error) {
				signed, err := signer.SignTRXTransfer(ctx, to, amountSun, "energy_rental")
				return signed.Raw, signed.TxID, err
			},
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return tronwallet.New(grid, signer, renter, logger, tronwallet.DefaultConfig(cfg.Network)), grid, nil
}

func waitForSigner(ctx context.Context, signer *tronwallet.SignerClient, logger *slog.Logger) (tron.Address, error) {
	for attempt := 1; ; attempt++ {
		treasury, err := signer.TreasuryAddress(ctx)
		if err == nil {
			return treasury, nil
		}
		if attempt >= 20 {
			return "", fmt.Errorf("reach signer: %w", err)
		}
		logger.Warn("waiting for signer", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

func WaitForSchema(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	expected, err := database.LatestEmbeddedVersion()
	if err != nil {
		return err
	}
	probe := database.SchemaReadiness(pool, expected)
	for {
		err := probe(ctx)
		if err == nil {
			return nil
		}
		logger.Warn("waiting for database schema", "expected_version", expected, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}
