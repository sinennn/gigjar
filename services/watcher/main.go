package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gigjar/internal/app"
	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/logging"
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

	if err := app.WaitForSchema(ctx, pool, logger); err != nil {
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

	w, err := app.NewWatcher(cfg, pool, logger)
	if err != nil {
		return err
	}

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
