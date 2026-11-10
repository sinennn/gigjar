package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gigjar/internal/app"
	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/logging"
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
	if err := app.WaitForSchema(ctx, pool, logger); err != nil {
		return err
	}
	sweeper, err := app.NewSweeper(ctx, cfg, pool, logger)
	if err != nil {
		return err
	}
	logger.Info("sweeper started", "threshold", cfg.SettlementThreshold.String(), "fee", cfg.SettlementFee.String(),
		"simulated", cfg.Network.IsSimulated(), "energy_provider", cfg.EnergyProvider)
	err = sweeper.Run(ctx)
	logger.Info("sweeper stopped")
	return err
}
