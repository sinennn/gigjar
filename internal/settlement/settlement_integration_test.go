//go:build integration

package settlement_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain"
	"gigjar/internal/chain/simulated"
	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/settlement"
	"gigjar/internal/testdb"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet"
	"gigjar/internal/wallet/simwallet"
	"gigjar/internal/watcher"
)

var (
	quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	cleva       = tron.Address("TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA")
)

type student struct {
	id      uuid.UUID
	address tron.Address
}

func newStudent(t *testing.T, pool *pgxpool.Pool, withDestination bool) student {
	t.Helper()
	id, address := testdb.CreateUserWithJar(t, pool)
	if withDestination {
		if _, err := jars.NewService(pool, usdt.MustParse("12"), usdt.MustParse("1")).SetDestination(context.Background(), id, cleva); err != nil {
			t.Fatal(err)
		}
	}
	return student{id: id, address: address}
}

func fund(t *testing.T, pool *pgxpool.Pool, s student, amounts ...string) {
	t.Helper()
	for _, amount := range amounts {
		if _, err := simulated.Send(context.Background(), pool, simulated.Transfer{To: s.address, Amount: usdt.MustParse(amount)}); err != nil {
			t.Fatal(err)
		}
	}
	w := watcher.New(pool, simulated.NewProvider(pool), quietLogger, watcher.DefaultConfig())
	makeDue(t, pool)
	if _, err := w.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	makeDue(t, pool)
	if _, err := w.ConfirmOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func makeDue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE jar_watch_state SET next_poll_at = now();
		UPDATE incoming_transfers SET next_check_at = now() WHERE status = 'pending';
		UPDATE settlements SET next_attempt_at = now() WHERE state IN ('queued', 'signing', 'broadcast');`)
	if err != nil {
		t.Fatal(err)
	}
}

func newSweeper(t *testing.T, pool *pgxpool.Pool, w wallet.Provider, c chain.Provider) *settlement.Sweeper {
	t.Helper()
	cfg := settlement.DefaultConfig()
	cfg.FailureCooldown = time.Hour
	s, err := settlement.New(pool, w, c, quietLogger, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func cycle(t *testing.T, pool *pgxpool.Pool, s *settlement.Sweeper) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.QueueOnce(ctx); err != nil {
		t.Fatal(err)
	}
	makeDue(t, pool)
	if _, err := s.AdvanceOnce(ctx); err != nil {
		t.Fatal(err)
	}
}

func balances(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) ledger.Balances {
	t.Helper()
	b, err := ledger.UserBalances(context.Background(), pool, userID)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func platformFees(t *testing.T, pool *pgxpool.Pool) usdt.Amount {
	t.Helper()
	var fees int64
	if err := pool.QueryRow(context.Background(), `SELECT balance FROM ledger_accounts WHERE kind = 'platform_fees'`).Scan(&fees); err != nil {
		t.Fatal(err)
	}
	return usdt.Amount(fees)
}

func onlySettlement(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) settlement.Settlement {
	t.Helper()
	page, err := settlement.List(context.Background(), pool, userID, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Settlements) != 1 {
		t.Fatalf("settlements = %d, want 1: %+v", len(page.Settlements), page.Settlements)
	}
	return page.Settlements[0]
}

func countSettlements(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM settlements WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertLedgerClean(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	discrepancies, err := ledger.Reconcile(context.Background(), pool)
	if err != nil || len(discrepancies) > 0 {
		t.Fatalf("ledger discrepancies = %+v, %v", discrepancies, err)
	}
}

func TestPitchDeckSweepEndToEnd(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "4", "3", "5")
	sweeper := newSweeper(t, pool, simwallet.New(pool, 0), simulated.NewProvider(pool))

	if _, err := sweeper.QueueOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	queued := onlySettlement(t, pool, s.id)
	if queued.State != settlement.StateQueued || queued.Amount != usdt.MustParse("12") || queued.Fee != usdt.MustParse("1") || queued.Destination != cleva {
		t.Fatalf("queued settlement = %+v", queued)
	}
	if b := balances(t, pool, s.id); b.Available != 0 || b.Reserved != usdt.MustParse("12") {
		t.Fatalf("balances while settling = %+v", b)
	}

	makeDue(t, pool)
	if _, err := sweeper.AdvanceOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	broadcast := onlySettlement(t, pool, s.id)
	if broadcast.State != settlement.StateBroadcast || broadcast.TxID == nil {
		t.Fatalf("after advance = %+v", broadcast)
	}

	makeDue(t, pool)
	if _, err := sweeper.AdvanceOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := onlySettlement(t, pool, s.id)
	if done.State != settlement.StateConfirmed || done.ConfirmedAt == nil || done.BlockNumber == nil {
		t.Fatalf("after confirmation = %+v", done)
	}
	if b := balances(t, pool, s.id); b != (ledger.Balances{Settled: usdt.MustParse("11")}) {
		t.Fatalf("final balances = %+v", b)
	}
	if fees := platformFees(t, pool); fees != usdt.MustParse("1") {
		t.Fatalf("platform fees = %s", fees)
	}
	clevaBalance, err := simulated.NewProvider(pool).TokenBalance(context.Background(), cleva)
	if err != nil || clevaBalance != usdt.MustParse("11") {
		t.Fatalf("cleva received %s, %v", clevaBalance, err)
	}
	jarBalance, err := simulated.NewProvider(pool).TokenBalance(context.Background(), s.address)
	if err != nil || jarBalance != usdt.MustParse("1") {
		t.Fatalf("jar keeps the fee on chain: %s, %v", jarBalance, err)
	}
	assertLedgerClean(t, pool)

	w := watcher.New(pool, simulated.NewProvider(pool), quietLogger, watcher.DefaultConfig())
	report, err := w.ReconcileOnce(context.Background())
	if err != nil || !report.Clean() || len(report.UnrecordedFunds) != 0 {
		t.Fatalf("watcher reconciliation after sweep = %+v, %v", report, err)
	}

	cycle(t, pool, sweeper)
	if countSettlements(t, pool, s.id) != 1 {
		t.Fatal("an empty jar must not be swept again")
	}
}

func TestJarsBelowThresholdOrWithoutDestinationAreNotSwept(t *testing.T) {
	pool := testdb.Open(t)
	poor := newStudent(t, pool, true)
	fund(t, pool, poor, "11.999999")
	nowhere := newStudent(t, pool, false)
	fund(t, pool, nowhere, "50")
	sweeper := newSweeper(t, pool, simwallet.New(pool, 0), simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	if countSettlements(t, pool, poor.id) != 0 || countSettlements(t, pool, nowhere.id) != 0 {
		t.Fatal("no sweep expected")
	}

	fund(t, pool, poor, "0.000001")
	cycle(t, pool, sweeper)
	if got := onlySettlement(t, pool, poor.id); got.Amount != usdt.MustParse("12") {
		t.Fatalf("settlement = %+v", got)
	}
}

func TestDepositsDuringASweepWaitForTheNextOne(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	sweeper := newSweeper(t, pool, simwallet.New(pool, time.Hour), simulated.NewProvider(pool))
	cycle(t, pool, sweeper)
	fund(t, pool, s, "20")
	cycle(t, pool, sweeper)

	if countSettlements(t, pool, s.id) != 1 {
		t.Fatal("only one sweep may be in flight per student")
	}
	if b := balances(t, pool, s.id); b.Available != usdt.MustParse("20") || b.Reserved != usdt.MustParse("12") {
		t.Fatalf("balances = %+v", b)
	}
}

type scriptedWallet struct {
	*simwallet.Wallet
	pool         *pgxpool.Pool
	signs        atomic.Int64
	broadcasts   atomic.Int64
	failFirstN   int64
	neverLand    bool
	revert       bool
	notReadyN    int64
	prepareCalls atomic.Int64
	prepareErr   error
	wrongAmount  bool
	expiresIn    time.Duration
}

func (w *scriptedWallet) PrepareSweep(ctx context.Context, p *wallet.Preparation) (wallet.Readiness, error) {
	call := w.prepareCalls.Add(1)
	if w.prepareErr != nil {
		return wallet.Readiness{}, w.prepareErr
	}
	if call <= w.notReadyN {
		required := int64(130285)
		p.EnergyRequired = &required
		p.Expenses = append(p.Expenses, wallet.Expense{Kind: "activation", TRXSun: 1_100_000, Reference: "activation-" + p.SettlementID.String()})
		return wallet.Readiness{Ready: false, RetryAfter: time.Millisecond, Note: "waiting for energy"}, nil
	}
	return w.Wallet.PrepareSweep(ctx, p)
}

func (w *scriptedWallet) SignSweep(ctx context.Context, r wallet.SweepRequest) (wallet.SignedTx, error) {
	w.signs.Add(1)
	if w.wrongAmount {
		r.NetAmount -= 1
	}
	signed, err := w.Wallet.SignSweep(ctx, r)
	if w.expiresIn != 0 {
		signed.ExpiresAt = time.Now().Add(w.expiresIn)
	}
	return signed, err
}

func (w *scriptedWallet) Broadcast(ctx context.Context, tx wallet.SignedTx) error {
	call := w.broadcasts.Add(1)
	if call <= w.failFirstN {
		return errors.New("node unavailable")
	}
	if w.neverLand {
		return nil
	}
	if err := w.Wallet.Broadcast(ctx, tx); err != nil {
		return err
	}
	if w.revert {
		_, err := w.pool.Exec(ctx, `UPDATE simulated_chain_transfers SET outcome = 'reverted' WHERE tx_hash = $1`, tx.TxID)
		return err
	}
	return nil
}

func TestCrashBetweenSigningAndBroadcastReusesTheSameTransaction(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "15")
	w := &scriptedWallet{Wallet: simwallet.New(pool, 0), pool: pool, failFirstN: 2}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	stuck := onlySettlement(t, pool, s.id)
	if stuck.State != settlement.StateSigning || stuck.TxID == nil {
		t.Fatalf("after failed broadcast = %+v", stuck)
	}
	for range 4 {
		cycle(t, pool, sweeper)
	}
	done := onlySettlement(t, pool, s.id)
	if done.State != settlement.StateConfirmed || *done.TxID != *stuck.TxID {
		t.Fatalf("settlement = %+v", done)
	}
	if w.signs.Load() != 1 {
		t.Fatalf("signed %d times; a persisted transaction must be reused", w.signs.Load())
	}
	if b := balances(t, pool, s.id); b.Settled != usdt.MustParse("14") || b.Reserved != 0 {
		t.Fatalf("balances = %+v", b)
	}
}

func TestRevertedSweepReturnsFundsAndCoolsDown(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	w := &scriptedWallet{Wallet: simwallet.New(pool, 0), pool: pool, revert: true}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	cycle(t, pool, sweeper)
	failed := onlySettlement(t, pool, s.id)
	if failed.State != settlement.StateFailed || failed.FailureReason == nil {
		t.Fatalf("settlement = %+v", failed)
	}
	if b := balances(t, pool, s.id); b != (ledger.Balances{Available: usdt.MustParse("12")}) {
		t.Fatalf("funds must return to the jar: %+v", b)
	}
	cycle(t, pool, sweeper)
	if countSettlements(t, pool, s.id) != 1 {
		t.Fatal("a failed jar must cool down before the next attempt")
	}
	if _, err := pool.Exec(context.Background(), `ALTER TABLE settlements DISABLE TRIGGER settlements_freeze; UPDATE settlements SET failed_at = now() - interval '2 hours'; ALTER TABLE settlements ENABLE TRIGGER settlements_freeze;`); err != nil {
		t.Fatal(err)
	}
	w.revert = false
	cycle(t, pool, sweeper)
	cycle(t, pool, sweeper)
	if b := balances(t, pool, s.id); b.Settled != usdt.MustParse("11") {
		t.Fatalf("retry after cooldown = %+v", b)
	}
	assertLedgerClean(t, pool)
}

func TestSweepThatNeverLandsIsReleasedOnlyAfterExpiry(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	w := &scriptedWallet{Wallet: simwallet.New(pool, 0), pool: pool, neverLand: true, expiresIn: time.Hour}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	cycle(t, pool, sweeper)
	if got := onlySettlement(t, pool, s.id); got.State != settlement.StateBroadcast {
		t.Fatalf("an unexpired transaction must not be released: %+v", got)
	}
	if b := balances(t, pool, s.id); b.Reserved != usdt.MustParse("12") {
		t.Fatalf("balances = %+v", b)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE settlements SET tx_expires_at = now() - interval '10 minutes'`); err != nil {
		t.Fatal(err)
	}
	cycle(t, pool, sweeper)
	got := onlySettlement(t, pool, s.id)
	if got.State != settlement.StateFailed || *got.FailureReason != settlement.ReasonDropped {
		t.Fatalf("settlement = %+v", got)
	}
	if b := balances(t, pool, s.id); b.Available != usdt.MustParse("12") {
		t.Fatalf("balances = %+v", b)
	}
}

func TestExpiredButKnownTransactionIsNeverReleased(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	w := &scriptedWallet{Wallet: simwallet.New(pool, time.Hour), pool: pool, expiresIn: -time.Hour}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	cycle(t, pool, sweeper)
	cycle(t, pool, sweeper)
	if got := onlySettlement(t, pool, s.id); got.State != settlement.StateBroadcast {
		t.Fatalf("a transaction the node knows about must keep being tracked: %+v", got)
	}
	if b := balances(t, pool, s.id); b.Reserved != usdt.MustParse("12") || b.Available != 0 {
		t.Fatalf("balances = %+v", b)
	}
}

func TestFinalTransactionWithUnexpectedTransferNeedsManualReview(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	w := &scriptedWallet{Wallet: simwallet.New(pool, 0), pool: pool, wrongAmount: true}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	cycle(t, pool, sweeper)
	got := onlySettlement(t, pool, s.id)
	if got.State != settlement.StateBroadcast {
		t.Fatalf("settlement = %+v", got)
	}
	var lastError string
	if err := pool.QueryRow(context.Background(), `SELECT last_error FROM settlements WHERE id = $1`, got.ID).Scan(&lastError); err != nil || lastError == "" {
		t.Fatalf("expected a manual review marker, got %q %v", lastError, err)
	}
	if b := balances(t, pool, s.id); b.Reserved != usdt.MustParse("12") {
		t.Fatalf("funds must stay reserved pending review: %+v", b)
	}
}

func TestPreparationWaitsThenProceedsAndRecordsExpenses(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	w := &scriptedWallet{Wallet: simwallet.New(pool, 0), pool: pool, notReadyN: 2}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	for range 4 {
		cycle(t, pool, sweeper)
	}
	got := onlySettlement(t, pool, s.id)
	if got.State != settlement.StateConfirmed || got.EnergyRequired == nil || *got.EnergyRequired != 130285 {
		t.Fatalf("settlement = %+v", got)
	}
	var expenses int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM settlement_expenses WHERE settlement_id = $1`, got.ID).Scan(&expenses); err != nil || expenses != 1 {
		t.Fatalf("expenses = %d, %v (the same expense reported twice must be stored once)", expenses, err)
	}
}

func TestPermanentPreparationErrorFailsAndReleases(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	w := &scriptedWallet{Wallet: simwallet.New(pool, 0), pool: pool, prepareErr: errors.Join(wallet.ErrPermanent, errors.New("jar account is a contract"))}
	sweeper := newSweeper(t, pool, w, simulated.NewProvider(pool))

	cycle(t, pool, sweeper)
	got := onlySettlement(t, pool, s.id)
	if got.State != settlement.StateFailed || *got.FailureReason != settlement.ReasonPreparationFailed {
		t.Fatalf("settlement = %+v", got)
	}
	if b := balances(t, pool, s.id); b.Available != usdt.MustParse("12") {
		t.Fatalf("balances = %+v", b)
	}
}

func TestParallelSweepersNeverDoubleSweep(t *testing.T) {
	pool := testdb.Open(t)
	var students []student
	for range 5 {
		s := newStudent(t, pool, true)
		fund(t, pool, s, "13")
		students = append(students, s)
	}

	var wait sync.WaitGroup
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			cfg := settlement.DefaultConfig()
			sweeper, err := settlement.New(pool, simwallet.New(pool, 0), simulated.NewProvider(pool), quietLogger, cfg)
			if err != nil {
				t.Error(err)
				return
			}
			for range 6 {
				if _, err := sweeper.QueueOnce(context.Background()); err != nil {
					t.Error(err)
				}
				if _, err := pool.Exec(context.Background(), `UPDATE settlements SET next_attempt_at = now() WHERE state IN ('queued', 'signing', 'broadcast')`); err != nil {
					t.Error(err)
				}
				if _, err := sweeper.AdvanceOnce(context.Background()); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wait.Wait()
	final := newSweeper(t, pool, simwallet.New(pool, 0), simulated.NewProvider(pool))
	for range 3 {
		cycle(t, pool, final)
	}

	for _, s := range students {
		if countSettlements(t, pool, s.id) != 1 {
			t.Fatalf("student %s has %d settlements", s.id, countSettlements(t, pool, s.id))
		}
		if b := balances(t, pool, s.id); b != (ledger.Balances{Settled: usdt.MustParse("12")}) {
			t.Fatalf("student %s balances = %+v", s.id, b)
		}
	}
	if fees := platformFees(t, pool); fees != usdt.MustParse("5") {
		t.Fatalf("platform fees = %s", fees)
	}
	var onChain int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM simulated_chain_transfers WHERE to_address = $1`, cleva).Scan(&onChain); err != nil || onChain != 5 {
		t.Fatalf("on-chain sweeps = %d, %v", onChain, err)
	}
	assertLedgerClean(t, pool)
}

func TestResolvedSettlementsAreImmutable(t *testing.T) {
	pool := testdb.Open(t)
	s := newStudent(t, pool, true)
	fund(t, pool, s, "12")
	sweeper := newSweeper(t, pool, simwallet.New(pool, 0), simulated.NewProvider(pool))
	for range 3 {
		cycle(t, pool, sweeper)
	}
	got := onlySettlement(t, pool, s.id)
	for _, statement := range []string{
		`UPDATE settlements SET destination_address = 'TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t' WHERE id = $1`,
		`UPDATE settlements SET state = 'failed', failed_at = now(), failure_reason = 'x' WHERE id = $1`,
		`DELETE FROM settlements WHERE id = $1`,
	} {
		if _, err := pool.Exec(context.Background(), statement, got.ID); err == nil {
			t.Errorf("%q must be rejected", statement)
		}
	}
}

func TestConfigRejectsThresholdsBelowClevaMinimum(t *testing.T) {
	cfg := settlement.DefaultConfig()
	cfg.Threshold = usdt.MustParse("11")
	if cfg.Validate() == nil {
		t.Fatal("threshold 11 with a 1 USDT fee nets exactly 10 and must be rejected")
	}
	cfg.Threshold = usdt.MustParse("11.000001")
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

type nileStub struct {
	*simwallet.Wallet
}

func (nileStub) Network() tron.Network { return tron.NetworkNile }

func (nileStub) JarAddress(ctx context.Context, index int64) (tron.Address, error) {
	return tron.SimulatedJarAddress(1_000_000 + index), nil
}

type nileChain struct{ chain.Provider }

func (nileChain) Network() tron.Network { return tron.NetworkNile }

func TestRealNetworkJarsAreProvisionedFromTheWallet(t *testing.T) {
	pool := testdb.Open(t)
	userID := testdb.CreateUser(t, pool)
	ctx := context.Background()
	if err := database.WithTx(ctx, pool, func(tx pgx.Tx) error { return jars.Create(ctx, tx, userID, tron.NetworkNile) }); err != nil {
		t.Fatal(err)
	}
	before, _ := jars.Get(ctx, pool, userID)
	if before.Address != nil {
		t.Fatal("real-network jars start without an address")
	}
	sweeper := newSweeper(t, pool, nileStub{simwallet.New(pool, 0)}, nileChain{simulated.NewProvider(pool)})
	if count, err := sweeper.ProvisionOnce(ctx); err != nil || count != 1 {
		t.Fatalf("provisioned %d, %v", count, err)
	}
	after, _ := jars.Get(ctx, pool, userID)
	if after.Address == nil || *after.Address != tron.SimulatedJarAddress(1_000_000+after.DerivationIndex) {
		t.Fatalf("jar = %+v", after)
	}
	var watched bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM jar_watch_state WHERE user_id = $1 AND network = 'tron-nile')`, userID).Scan(&watched); err != nil || !watched {
		t.Fatal("a provisioned jar must be watched")
	}
	if count, _ := sweeper.ProvisionOnce(ctx); count != 0 {
		t.Fatal("provisioning must be idempotent")
	}
}

func TestMismatchedNetworksAreRefused(t *testing.T) {
	pool := testdb.Open(t)
	if _, err := settlement.New(pool, nileStub{simwallet.New(pool, 0)}, simulated.NewProvider(pool), quietLogger, settlement.DefaultConfig()); err == nil {
		t.Fatal("expected network mismatch error")
	}
}
