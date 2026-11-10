//go:build integration

package database

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

func integrationDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("GIGJAR_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("GIGJAR_TEST_DATABASE_URL must be set for integration tests")
	}
	return url
}

func TestMigrationsApplyRevertAndReapply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	url := integrationDatabaseURL(t)

	migrator, err := NewMigrator(url, logger)
	if err != nil {
		t.Fatalf("new migrator: %v", err)
	}
	defer migrator.Close()

	expected, err := LatestEmbeddedVersion()
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatalf("up: %v", err)
	}
	for step := expected; step > 0; step-- {
		if err := migrator.DownOne(); err != nil {
			t.Fatalf("down from %d: %v", step, err)
		}
	}
	if err := migrator.Up(); err != nil {
		t.Fatalf("re-apply: %v", err)
	}

	pool, err := Connect(ctx, url, DefaultConnectOptions(), logger)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := SchemaReadiness(pool, expected)(ctx); err != nil {
		t.Fatalf("schema not ready after migrations: %v", err)
	}
}

func TestForbidMutationTriggerRejectsUpdatesAndDeletes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	url := integrationDatabaseURL(t)

	migrator, err := NewMigrator(url, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	migrator.Close()

	pool, err := Connect(ctx, url, DefaultConnectOptions(), logger)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	statements := []string{
		`CREATE TEMP TABLE immutable_probe (id int PRIMARY KEY, value text) ON COMMIT DROP`,
		`CREATE TRIGGER immutable_probe_guard BEFORE UPDATE OR DELETE ON immutable_probe FOR EACH ROW EXECUTE FUNCTION gigjar_forbid_mutation()`,
		`INSERT INTO immutable_probe VALUES (1, 'original')`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	if _, err := tx.Exec(ctx, `SAVEPOINT before_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE immutable_probe SET value = 'changed'`); err == nil {
		t.Error("update succeeded on an immutable table")
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM immutable_probe`); err == nil {
		t.Error("delete succeeded on an immutable table")
	}
}
