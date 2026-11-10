//go:build integration && trongrid

package watcher_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"gigjar/internal/chain/trongrid"
	"gigjar/internal/database"
	"gigjar/internal/ledger"
	"gigjar/internal/testdb"
	"gigjar/internal/transfers"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/watcher"
)

const (
	nileRecipient tron.Address = "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA"
	nileTxHash                 = "ce8eb3c878ce2d1a435c1990e39aa021da835c57a8547f404e499474b799c7a1"
)

func TestLiveNileWatcherCreditsARealSolidifiedTransfer(t *testing.T) {
	key := os.Getenv("GIGJAR_TRONGRID_API_KEY")
	if key == "" {
		t.Fatal("GIGJAR_TRONGRID_API_KEY must be set")
	}
	pool := testdb.Open(t)
	ctx := context.Background()
	student := testdb.CreateUser(t, pool)
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO jars (user_id, network) VALUES ($1, 'tron-nile')`, student); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE jars SET address = $2, provisioned_at = now() WHERE user_id = $1`, student, nileRecipient); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO jar_watch_state (user_id, network, address, min_block_timestamp_ms)
			VALUES ($1, 'tron-nile', $2, 1794159243000)`, student, nileRecipient)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	baseURL, _ := trongrid.DefaultBaseURL(tron.NetworkNile)
	provider, err := trongrid.New(trongrid.Config{BaseURL: baseURL, APIKey: key, Network: tron.NetworkNile, MaxPages: 2})
	if err != nil {
		t.Fatal(err)
	}
	cfg := watcher.DefaultConfig()
	cfg.CursorOverlap = time.Second
	w := watcher.New(pool, provider, quietLogger, cfg)

	if _, err := w.DiscoverOnce(ctx); err != nil {
		t.Fatal(err)
	}
	makeDue(t, pool)
	if _, err := w.ConfirmOnce(ctx); err != nil {
		t.Fatal(err)
	}

	got := transferByHash(t, pool, nileTxHash)
	if got.Status != transfers.StatusConfirmed || got.Amount != usdt.MustParse("1000") || got.BlockNumber == nil || *got.BlockNumber != 72540166 {
		t.Fatalf("transfer = %+v", got)
	}
	b, err := ledger.UserBalances(ctx, pool, student)
	if err != nil {
		t.Fatal(err)
	}
	if b.Available < usdt.MustParse("1000") {
		t.Fatalf("balances = %+v", b)
	}
	discrepancies, err := ledger.Reconcile(ctx, pool)
	if err != nil || len(discrepancies) != 0 {
		t.Fatalf("ledger discrepancies = %+v, %v", discrepancies, err)
	}
	t.Logf("credited %s from %s on Nile block %d", got.Amount, got.TxHash, *got.BlockNumber)
}
