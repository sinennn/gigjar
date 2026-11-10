package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gigjar/internal/app"
	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gigjar:", err)
		os.Exit(1)
	}
}

func run() error {
	apiConfig, err := config.LoadAPI(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load api config: %w", err)
	}
	watcherConfig, err := config.LoadWatcher(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load watcher config: %w", err)
	}
	sweeperConfig, err := config.LoadSweeper(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load sweeper config: %w", err)
	}
	base := logging.New(os.Stdout, apiConfig.LogLevel, apiConfig.LogFormat).With("network", string(apiConfig.Network))
	logger := base.With("service", "gigjar")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	migrator, err := database.NewMigrator(apiConfig.DatabaseURL, base.With("service", "migrate"))
	if err != nil {
		return err
	}
	if err := migrator.Up(); err != nil {
		migrator.Close()
		return fmt.Errorf("migrate: %w", err)
	}
	migrator.Close()

	pool, err := database.Connect(ctx, apiConfig.DatabaseURL, database.DefaultConnectOptions(), logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	if apiConfig.Network.IsSimulated() {
		if _, err := jars.ProvisionMissingSimulated(ctx, pool); err != nil {
			return err
		}
	}

	handler, err := app.NewAPIHandler(apiConfig, pool, base.With("service", "api"))
	if err != nil {
		return err
	}
	chainWatcher, err := app.NewWatcher(watcherConfig, pool, base.With("service", "watcher"))
	if err != nil {
		return err
	}
	sweeper, err := app.NewSweeper(ctx, sweeperConfig, pool, base.With("service", "sweeper"))
	if err != nil {
		return err
	}

	server := &http.Server{Addr: apiConfig.HTTPAddr, ReadHeaderTimeout: apiConfig.ReadHeaderTimeout, Handler: handler}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	failures := make(chan error, 3)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- fmt.Errorf("http server: %w", err)
		}
	}()
	go func() {
		if err := chainWatcher.Run(runCtx); err != nil {
			failures <- fmt.Errorf("watcher: %w", err)
		}
	}()
	go func() {
		if err := sweeper.Run(runCtx); err != nil {
			failures <- fmt.Errorf("sweeper: %w", err)
		}
	}()
	logger.Info("gigjar started", "addr", apiConfig.HTTPAddr, "environment", apiConfig.Environment,
		"threshold", sweeperConfig.SettlementThreshold.String(), "fee", sweeperConfig.SettlementFee.String())

	var failure error
	select {
	case <-ctx.Done():
	case failure = <-failures:
		logger.Error("component failed; shutting down", "error", failure)
	}
	cancel()
	shutdownCtx, release := context.WithTimeout(context.Background(), apiConfig.ShutdownTimeout)
	defer release()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Join(failure, fmt.Errorf("graceful shutdown: %w", err))
	}
	logger.Info("gigjar stopped")
	return failure
}
