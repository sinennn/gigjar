//go:build integration && trongrid

package settlement_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"gigjar/internal/chain/trongrid"
	"gigjar/internal/database"
	"gigjar/internal/energy/tronsave"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/settlement"
	"gigjar/internal/testdb"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet/tronwallet"
	"gigjar/internal/watcher"
)

const (
	testClientIndex      = 1_999_999_999
	testDestinationIndex = 1_999_999_998
)

func TestLiveNileSweepEndToEnd(t *testing.T) {
	key, token, signerURL := os.Getenv("GIGJAR_TRONGRID_API_KEY"), os.Getenv("GIGJAR_SIGNER_TOKEN"), os.Getenv("GIGJAR_SIGNER_URL")
	if key == "" || token == "" || signerURL == "" {
		t.Fatal("GIGJAR_TRONGRID_API_KEY, GIGJAR_SIGNER_TOKEN and GIGJAR_SIGNER_URL must be set")
	}
	ctx := context.Background()
	pool := testdb.Open(t)
	logger := testLogger(t)

	baseURL, _ := trongrid.DefaultBaseURL(tron.NetworkNile)
	grid, err := trongrid.New(trongrid.Config{BaseURL: baseURL, APIKey: key, Network: tron.NetworkNile})
	if err != nil {
		t.Fatal(err)
	}
	signer := tronwallet.NewSignerClient(signerURL, token)
	treasury, err := signer.TreasuryAddress(ctx)
	if err != nil {
		t.Fatalf("signer unreachable: %v", err)
	}
	client, err := signer.JarAddress(ctx, testClientIndex)
	if err != nil {
		t.Fatal(err)
	}
	destination, err := signer.JarAddress(ctx, testDestinationIndex)
	if err != nil {
		t.Fatal(err)
	}
	treasuryResources, err := grid.AccountResources(ctx, treasury)
	if err != nil {
		t.Fatal(err)
	}
	clientUSDT, err := grid.TokenBalance(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("treasury %s holds %.6f TRX; test client %s holds %s USDT", treasury, float64(treasuryResources.BalanceSun)/1e6, client, clientUSDT)
	if treasuryResources.BalanceSun < 40_000_000 || clientUSDT < usdt.MustParse("12") {
		t.Fatalf("fund the treasury with Nile TRX and the test client with Nile USDT from https://nileex.io/join/getJoinPage first")
	}

	userID := testdb.CreateUser(t, pool)
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) error { return jars.Create(ctx, tx, userID, tron.NetworkNile) }); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `SELECT setval('jar_derivation_index_seq', (extract(epoch FROM now()))::bigint)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jars SET derivation_index = nextval('jar_derivation_index_seq') WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := jars.NewService(pool, usdt.MustParse("12"), usdt.MustParse("1")).SetDestination(ctx, userID, destination); err != nil {
		t.Fatal(err)
	}

	renterURL, paymentAddress, _ := tronsave.Defaults(tron.NetworkNile)
	renter, err := tronsave.New(tronsave.Config{
		BaseURL: renterURL, PaymentAddress: paymentAddress, DurationSeconds: 900, MaxUnitPriceSun: 99,
		Pay: func(ctx context.Context, to tron.Address, amountSun int64) ([]byte, string, error) {
			signed, err := signer.SignTRXTransfer(ctx, to, amountSun, "energy_rental")
			return signed.Raw, signed.TxID, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	walletCfg := tronwallet.DefaultConfig(tron.NetworkNile)
	realWallet := tronwallet.New(grid, signer, renter, logger, walletCfg)
	sweeperCfg := settlement.DefaultConfig()
	sweeperCfg.PollInterval = 3 * time.Second
	sweeper, err := settlement.New(pool, realWallet, grid, logger, sweeperCfg)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := sweeper.ProvisionOnce(ctx); err != nil || count != 1 {
		t.Fatalf("provisioned %d, %v", count, err)
	}
	jar, _ := jars.Get(ctx, pool, userID)
	t.Logf("jar address %s (derivation index %d)", *jar.Address, jar.DerivationIndex)

	clientResources, err := grid.AccountResources(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	if clientResources.BalanceSun < 10_000_000 {
		topUp, err := signer.SignTRXTransfer(ctx, client, 10_000_000, "test_client")
		if err != nil {
			t.Fatal(err)
		}
		if err := grid.Broadcast(ctx, topUp.Raw); err != nil {
			t.Fatal(err)
		}
		t.Logf("treasury sent 10 TRX to the test client for gas: %s", topUp.TxID)
		waitFor(t, 90*time.Second, func() bool {
			r, err := grid.AccountResources(ctx, client)
			return err == nil && r.BalanceSun >= 10_000_000
		})
	}
	payment, _, err := signer.SignUSDTTransfer(ctx, testClientIndex, *jar.Address, usdt.MustParse("12"), 30_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := grid.Broadcast(ctx, payment.Raw); err != nil {
		t.Fatal(err)
	}
	t.Logf("client paid 12 USDT to the jar: %s", payment.TxID)

	watcherCfg := watcher.DefaultConfig()
	watcherCfg.CursorOverlap = time.Hour
	w := watcher.New(pool, grid, logger, watcherCfg)
	started := time.Now()
	waitFor(t, 5*time.Minute, func() bool {
		makeDue(t, pool)
		if _, err := w.DiscoverOnce(ctx); err != nil {
			t.Logf("discover: %v", err)
		}
		makeDue(t, pool)
		if _, err := w.ConfirmOnce(ctx); err != nil {
			t.Logf("confirm: %v", err)
		}
		b, _ := ledger.UserBalances(ctx, pool, userID)
		return b.Available == usdt.MustParse("12")
	})
	t.Logf("watcher credited the payment after %s", time.Since(started).Round(time.Second))

	started = time.Now()
	waitFor(t, 10*time.Minute, func() bool {
		if _, err := sweeper.QueueOnce(ctx); err != nil {
			t.Logf("queue: %v", err)
		}
		if _, err := sweeper.AdvanceOnce(ctx); err != nil {
			t.Logf("advance: %v", err)
		}
		page, _ := settlement.List(ctx, pool, userID, "", 1)
		if len(page.Settlements) == 0 {
			return false
		}
		s := page.Settlements[0]
		if s.State == settlement.StateFailed {
			t.Fatalf("sweep failed: %s", *s.FailureReason)
		}
		return s.State == settlement.StateConfirmed
	})
	page, _ := settlement.List(ctx, pool, userID, "", 1)
	done := page.Settlements[0]
	t.Logf("sweep confirmed after %s: tx %s in block %d", time.Since(started).Round(time.Second), *done.TxID, *done.BlockNumber)

	b, _ := ledger.UserBalances(ctx, pool, userID)
	if b != (ledger.Balances{Settled: usdt.MustParse("11")}) {
		t.Fatalf("balances = %+v", b)
	}
	received, err := grid.TokenBalance(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("destination %s now holds %s USDT on Nile", destination, received)
	if received < usdt.MustParse("11") {
		t.Fatalf("destination received %s", received)
	}

	rows, err := pool.Query(ctx, `SELECT kind, trx_sun, reference, detail FROM settlement_expenses WHERE settlement_id = $1 ORDER BY id`, done.ID)
	if err != nil {
		t.Fatal(err)
	}
	var totalSun int64
	for rows.Next() {
		var (
			kind, reference string
			sun             int64
			detail          json.RawMessage
		)
		if err := rows.Scan(&kind, &sun, &reference, &detail); err != nil {
			t.Fatal(err)
		}
		totalSun += sun
		t.Logf("expense %-14s %10.6f TRX  %s %s", kind, float64(sun)/1e6, reference, detail)
	}
	rows.Close()
	t.Logf("platform TRX spent on this sweep: %.6f TRX (energy required %d, burn fallback %v)", float64(totalSun)/1e6, *done.EnergyRequired, done.BurnFallback)
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("condition not met within %s", timeout)
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
