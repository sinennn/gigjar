package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gigjar/internal/config"
	"gigjar/internal/database"
	"gigjar/internal/httpapi"
	"gigjar/internal/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAPI(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat).With("service", "api")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, database.DefaultConnectOptions(), logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	expectedSchema, err := database.LatestEmbeddedVersion()
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		Handler: httpapi.NewRouter(httpapi.RouterConfig{
			Logger:         logger,
			AllowedOrigins: cfg.CORSAllowedOrigins,
			RequestTimeout: cfg.RequestTimeout,
			Readiness:      database.SchemaReadiness(pool, expectedSchema),
		}),
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "environment", cfg.Environment)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("http server stopped")
	return nil
}
