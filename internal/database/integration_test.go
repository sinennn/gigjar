//go:build integration

package database

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func integrationDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("GIGJAR_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("GIGJAR_TEST_DATABASE_URL must be set for integration tests")
	}
	return url
}

func scratchDatabase(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	base := integrationDatabaseURL(t)
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { admin.Close(context.Background()) })
	name := fmt.Sprintf("gigjar_migrations_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create scratch database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)"); err != nil {
			t.Errorf("drop scratch database: %v", err)
		}
	})
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

func TestMigrationsApplyRevertAndReapply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	url := scratchDatabase(t)

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

func TestRollbackRefusesToDestroySettlements(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	url := scratchDatabase(t)
	migrator, err := NewMigrator(url, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `
		WITH u AS (INSERT INTO users (email, password_hash, display_name) VALUES ('guard@test.gigjar', 'x', 'Guard') RETURNING id),
		     d AS (INSERT INTO cleva_destinations (user_id, address) SELECT id, 'TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA' FROM u RETURNING id, user_id, address)
		INSERT INTO settlements (user_id, network, jar_address, jar_index, destination_id, destination_address, amount, fee)
		SELECT user_id, 'simulated', address, 1, id, address, 12000000, 1000000 FROM d`)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrator.DownOne(); err != nil {
		t.Fatalf("rolling back 000008 without google-only users must work: %v", err)
	}
	err = migrator.DownOne()
	if err == nil {
		t.Fatal("rolling back settlements with data must be refused")
	}
	var settlements int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM settlements`).Scan(&settlements); err != nil || settlements != 1 {
		t.Fatalf("settlement data must be untouched: %d, %v", settlements, err)
	}
}

func TestRollbackRefusesToStrandGoogleOnlyUsers(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	url := scratchDatabase(t)
	migrator, err := NewMigrator(url, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer migrator.Close()
	if err := migrator.Up(); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `INSERT INTO users (email, password_hash, display_name) VALUES ('google-only@test.gigjar', NULL, 'G')`); err != nil {
		t.Fatal(err)
	}
	if err := migrator.DownOne(); err == nil {
		t.Fatal("rolling back while google-only users exist must be refused")
	}
}
