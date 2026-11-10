package main

import (
	"context"
	"errors"
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
	"gigjar/internal/jars"
	"gigjar/internal/logging"
	"gigjar/internal/watcher"
)

const usage = "usage: watcher [run|reconcile]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "watcher:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "run"
	if len(args) > 0 {
		command = args[0]
	}
	if command != "run" && command != "reconcile" {
		return errors.New(usage)
	}

	cfg, err := config.LoadWatcher(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat).With("service", "watcher", "network", string(cfg.Network))

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

	provider, err := newProvider(cfg, pool)
	if err != nil {
		return err
	}

	if cfg.Network.IsSimulated() {
		count, err := jars.ProvisionMissingSimulated(ctx, pool)
		if err != nil {
			return err
		}
		if count > 0 {
			logger.Info("provisioned simulated jar addresses", "count", count)
		}
	}

	watcherConfig := watcher.DefaultConfig()
	watcherConfig.Tick = cfg.Tick
	watcherConfig.BatchSize = cfg.BatchSize
	watcherConfig.ActivePollInterval = cfg.ActivePollInterval
	watcherConfig.IdlePollInterval = cfg.IdlePollInterval
	watcherConfig.CursorOverlap = cfg.CursorOverlap
	watcherConfig.DropAfter = cfg.DropAfter
	watcherConfig.ReconcileInterval = cfg.ReconcileInterval
	w := watcher.New(pool, provider, logger, watcherConfig)

	if command == "reconcile" {
		report, err := w.ReconcileOnce(ctx)
		if err != nil {
			return err
		}
		report.Log(logger)
		if !report.Clean() {
			return errors.New("reconciliation found discrepancies")
		}
		return nil
	}

	logger.Info("watcher started", "simulated", cfg.Network.IsSimulated(), "active_poll_interval", cfg.ActivePollInterval.String())
	err = w.Run(ctx)
	logger.Info("watcher stopped")
	return err
}

func newProvider(cfg config.Watcher, pool *pgxpool.Pool) (chain.Provider, error) {
	if cfg.Network.IsSimulated() {
		return simulated.NewProvider(pool), nil
	}
	return trongrid.New(trongrid.Config{BaseURL: cfg.TronGridURL, APIKey: cfg.TronGridAPIKey, Network: cfg.Network})
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
