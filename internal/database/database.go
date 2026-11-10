package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ConnectOptions struct {
	MaxConns        int32
	ConnectAttempts int
	RetryDelay      time.Duration
}

func DefaultConnectOptions() ConnectOptions {
	return ConnectOptions{
		MaxConns:        10,
		ConnectAttempts: 10,
		RetryDelay:      time.Second,
	}
}

func Connect(ctx context.Context, databaseURL string, options ConnectOptions, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if options.MaxConns > 0 {
		poolConfig.MaxConns = options.MaxConns
	}
	poolConfig.ConnConfig.RuntimeParams["timezone"] = "UTC"

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	attempts := max(options.ConnectAttempts, 1)
	for attempt := 1; ; attempt++ {
		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		if attempt >= attempts {
			pool.Close()
			return nil, fmt.Errorf("database unreachable after %d attempts: %w", attempts, err)
		}
		logger.Warn("database not ready, retrying", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(options.RetryDelay):
		}
	}
}

type SchemaStatus struct {
	Version int64
	Dirty   bool
}

func ReadSchemaStatus(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (SchemaStatus, error) {
	var status SchemaStatus
	err := querier.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&status.Version, &status.Dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return SchemaStatus{}, nil
	}
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("read schema status: %w", err)
	}
	return status, nil
}

func SchemaReadiness(pool *pgxpool.Pool, expectedVersion int64) func(context.Context) error {
	return func(ctx context.Context) error {
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("ping database: %w", err)
		}
		status, err := ReadSchemaStatus(ctx, pool)
		if err != nil {
			return err
		}
		return status.Check(expectedVersion)
	}
}

func (s SchemaStatus) Check(expectedVersion int64) error {
	if s.Dirty {
		return fmt.Errorf("schema version %d is dirty", s.Version)
	}
	if s.Version != expectedVersion {
		return fmt.Errorf("schema version %d does not match expected version %d", s.Version, expectedVersion)
	}
	return nil
}
