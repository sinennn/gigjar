package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain"
	"gigjar/internal/chain/simulated"
	"gigjar/internal/chain/trongrid"
	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/energy"
	"gigjar/internal/energy/tronsave"
	"gigjar/internal/logging"
	"gigjar/internal/settlement"
	"gigjar/internal/tron"
	"gigjar/internal/wallet"
	"gigjar/internal/wallet/simwallet"
	"gigjar/internal/wallet/tronwallet"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sweeper:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadSweeper(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat).With("service", "sweeper", "network", string(cfg.Network))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, database.DefaultConnectOptions(), logger)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := waitForSchema(ctx, pool, logger); err != nil {
		return err
	}

	w, c, err := providers(ctx, cfg, pool, logger)
	if err != nil {
		return err
	}

	sweeperConfig := settlement.DefaultConfig()
	sweeperConfig.Threshold = cfg.SettlementThreshold
	sweeperConfig.Fee = cfg.SettlementFee
	sweeperConfig.Tick = cfg.Tick
	sweeperConfig.BatchSize = cfg.BatchSize
	sweeper, err := settlement.New(pool, w, c, logger, sweeperConfig)
	if err != nil {
		return err
	}
	logger.Info("sweeper started", "threshold", cfg.SettlementThreshold.String(), "fee", cfg.SettlementFee.String(),
		"simulated", cfg.Network.IsSimulated(), "energy_provider", cfg.EnergyProvider)
	err = sweeper.Run(ctx)
	logger.Info("sweeper stopped")
	return err
}

func providers(ctx context.Context, cfg config.Sweeper, pool *pgxpool.Pool, logger *slog.Logger) (wallet.Provider, chain.Provider, error) {
	if cfg.Network.IsSimulated() {
		return simwallet.New(pool, cfg.SimulatedConfirmDelay), simulated.NewProvider(pool), nil
	}
	grid, err := trongrid.New(trongrid.Config{BaseURL: cfg.TronGridURL, APIKey: cfg.TronGridAPIKey, Network: cfg.Network})
	if err != nil {
		return nil, nil, err
	}
	signer := tronwallet.NewSignerClient(cfg.SignerURL, cfg.SignerToken)
	treasury, err := signer.TreasuryAddress(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reach signer: %w", err)
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

func waitForSchema(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
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
