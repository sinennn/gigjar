package database

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"gigjar/migrations"
)

type Migrator struct {
	migrate *migrate.Migrate
	logger  *slog.Logger
}

func NewMigrator(databaseURL string, logger *slog.Logger) (*Migrator, error) {
	source, err := iofs.New(migrations.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("open migration source: %w", err)
	}
	instance, err := migrate.NewWithSourceInstance("iofs", source, migrateDriverURL(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("open migration target: %w", err)
	}
	return &Migrator{migrate: instance, logger: logger}, nil
}

func (m *Migrator) Up() error {
	err := m.migrate.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		m.logger.Info("schema already up to date")
		return nil
	}
	if err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func (m *Migrator) DownOne() error {
	if err := m.migrate.Steps(-1); err != nil {
		return fmt.Errorf("revert migration: %w", err)
	}
	return nil
}

func (m *Migrator) Version() (SchemaStatus, error) {
	version, dirty, err := m.migrate.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return SchemaStatus{}, nil
	}
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("read migration version: %w", err)
	}
	return SchemaStatus{Version: int64(version), Dirty: dirty}, nil
}

func (m *Migrator) Close() error {
	sourceErr, databaseErr := m.migrate.Close()
	return errors.Join(sourceErr, databaseErr)
}

func migrateDriverURL(databaseURL string) string {
	for _, prefix := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(databaseURL, prefix) {
			return "pgx5://" + strings.TrimPrefix(databaseURL, prefix)
		}
	}
	return databaseURL
}

func LatestEmbeddedVersion() (int64, error) {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("list embedded migrations: %w", err)
	}
	var latest int64
	for _, entry := range entries {
		prefix, _, found := strings.Cut(entry.Name(), "_")
		if !found || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("migration %q has a non-numeric version: %w", entry.Name(), err)
		}
		latest = max(latest, version)
	}
	return latest, nil
}
