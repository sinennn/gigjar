package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/tron"
)

var migrateOnce sync.Once

func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("GIGJAR_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("GIGJAR_TEST_DATABASE_URL must be set for integration tests")
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var migrateErr error
	migrateOnce.Do(func() {
		migrator, err := database.NewMigrator(url, logger)
		if err != nil {
			migrateErr = err
			return
		}
		defer migrator.Close()
		migrateErr = migrator.Up()
	})
	if migrateErr != nil {
		t.Fatalf("migrate test database: %v", migrateErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	options := database.DefaultConnectOptions()
	options.MaxConns = 20
	pool, err := database.Connect(ctx, url, options, logger)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	Reset(t, pool)
	return pool
}

func Reset(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE postings, journal_entries, incoming_transfers, simulated_chain_transfers, jar_watch_state, payment_links, cleva_destinations, jars, sessions RESTART IDENTITY CASCADE;
		ALTER TABLE ledger_accounts DISABLE TRIGGER ledger_accounts_no_delete;
		DELETE FROM ledger_accounts WHERE user_id IS NOT NULL;
		UPDATE ledger_accounts SET balance = 0;
		ALTER TABLE ledger_accounts ENABLE TRIGGER ledger_accounts_no_delete;
		DELETE FROM users;`)
	if err != nil {
		t.Fatalf("reset test database: %v", err)
	}
}

func CreateUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var userID uuid.UUID
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, display_name) VALUES ($1, 'not-a-real-hash', 'Test Student') RETURNING id`,
			uuid.NewString()+"@test.gigjar",
		).Scan(&userID)
		if err != nil {
			return err
		}
		return ledger.OpenUserAccounts(ctx, tx, userID)
	})
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return userID
}

func CreateUserWithJar(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, tron.Address) {
	t.Helper()
	userID := CreateUser(t, pool)
	ctx := context.Background()
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return jars.Create(ctx, tx, userID, tron.NetworkSimulated)
	})
	if err != nil {
		t.Fatalf("create test jar: %v", err)
	}
	jar, err := jars.Get(ctx, pool, userID)
	if err != nil || jar.Address == nil {
		t.Fatalf("load test jar: %v", err)
	}
	return userID, *jar.Address
}

func RandomTxHash(t *testing.T) string {
	t.Helper()
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(random)
}

func IncomingTransfer(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO incoming_transfers (network, tx_hash, user_id, to_address, from_address, amount)
		VALUES ('simulated', $1, $2, $3, $3, 1)
		RETURNING id`,
		RandomTxHash(t), userID, tron.SimulatedJarAddress(0),
	).Scan(&id)
	if err != nil {
		t.Fatalf("create test incoming transfer: %v", err)
	}
	return id
}
