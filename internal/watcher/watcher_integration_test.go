//go:build integration

package watcher_test

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
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain"
	"gigjar/internal/chain/simulated"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/paylinks"
	"gigjar/internal/testdb"
	"gigjar/internal/transfers"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/watcher"
)

var quietLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func newWatcher(pool *pgxpool.Pool, provider chain.Provider) *watcher.Watcher {
	return watcher.New(pool, provider, quietLogger, watcher.DefaultConfig())
}

func makeDue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE jar_watch_state SET next_poll_at = now();
		UPDATE incoming_transfers SET next_check_at = now() WHERE status = 'pending';`)
	if err != nil {
		t.Fatal(err)
	}
}

func cycle(t *testing.T, pool *pgxpool.Pool, w *watcher.Watcher) {
	t.Helper()
	makeDue(t, pool)
	if _, err := w.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	makeDue(t, pool)
	if _, err := w.ConfirmOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func send(t *testing.T, pool *pgxpool.Pool, to tron.Address, amount string, delay time.Duration, outcome string) simulated.SentTransfer {
	t.Helper()
	sent, err := simulated.Send(context.Background(), pool, simulated.Transfer{To: to, Amount: usdt.MustParse(amount), ConfirmationDelay: delay, Outcome: outcome})
	if err != nil {
		t.Fatal(err)
	}
	return sent
}

func solidifyAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE simulated_chain_transfers SET solidifies_at = created_at`); err != nil {
		t.Fatal(err)
	}
}

func createLink(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, amount string) paylinks.Link {
	t.Helper()
	link, _, err := paylinks.NewService(pool).Create(context.Background(), userID, paylinks.CreateInput{Title: "Gig " + amount, Amount: usdt.MustParse(amount)})
	if err != nil {
		t.Fatal(err)
	}
	return link
}

func balances(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) (ledger.Balances, usdt.Amount) {
	t.Helper()
	b, err := ledger.UserBalances(context.Background(), pool, userID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := transfers.PendingTotal(context.Background(), pool, userID)
	if err != nil {
		t.Fatal(err)
	}
	return b, pending
}

func transferByHash(t *testing.T, pool *pgxpool.Pool, hash string) transfers.Transfer {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM incoming_transfers WHERE tx_hash = $1`, hash).Scan(&id); err != nil {
		t.Fatalf("transfer %s not recorded: %v", hash, err)
	}
	transfer, err := transfers.Get(context.Background(), pool, id)
	if err != nil {
		t.Fatal(err)
	}
	return transfer
}

func assertClean(t *testing.T, pool *pgxpool.Pool, w *watcher.Watcher) watcher.Report {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE jar_watch_state SET last_reconciled_at = NULL`); err != nil {
		t.Fatal(err)
	}
	report, err := w.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Clean() {
		t.Fatalf("reconciliation is not clean: %+v", report)
	}
	return report
}

func TestPendingThenConfirmedCreditsJarAndPaysLink(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	link := createLink(t, pool, student, "4")
	w := newWatcher(pool, simulated.NewProvider(pool))

	sent := send(t, pool, address, "4", time.Hour, "")
	cycle(t, pool, w)

	b, pending := balances(t, pool, student)
	if pending != usdt.MustParse("4") || b.Available != 0 {
		t.Fatalf("after observation: balances = %+v pending = %s", b, pending)
	}
	if got := transferByHash(t, pool, sent.TxHash); got.Status != transfers.StatusPending {
		t.Fatalf("transfer = %+v", got)
	}

	solidifyAll(t, pool)
	cycle(t, pool, w)

	b, pending = balances(t, pool, student)
	if pending != 0 || b.Available != usdt.MustParse("4") {
		t.Fatalf("after confirmation: balances = %+v pending = %s", b, pending)
	}
	confirmed := transferByHash(t, pool, sent.TxHash)
	if confirmed.Status != transfers.StatusConfirmed || confirmed.PaymentLinkID == nil || *confirmed.PaymentLinkID != link.ID || confirmed.BlockNumber == nil {
		t.Fatalf("confirmed transfer = %+v", confirmed)
	}
	paid, err := paylinks.NewService(pool).Get(context.Background(), student, link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.Status != paylinks.StatusPaid || paid.AmountReceived != usdt.MustParse("4") {
		t.Fatalf("link = %+v", paid)
	}

	cycle(t, pool, w)
	cycle(t, pool, w)
	if b, _ := balances(t, pool, student); b.Available != usdt.MustParse("4") {
		t.Fatalf("re-polling must not credit again: %+v", b)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM incoming_transfers`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("incoming transfer rows = %d, %v", count, err)
	}
	report := assertClean(t, pool, w)
	if report.JarsChecked != 1 || len(report.UnrecordedFunds) != 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestPitchDeckJarFillsFromThreeGigs(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	flyer := createLink(t, pool, student, "4")
	tutoring := createLink(t, pool, student, "3")
	w := newWatcher(pool, simulated.NewProvider(pool))

	for _, amount := range []string{"4", "3", "5"} {
		send(t, pool, address, amount, 0, "")
	}
	cycle(t, pool, w)

	b, pending := balances(t, pool, student)
	if b.Available != usdt.MustParse("12") || pending != 0 {
		t.Fatalf("balances = %+v pending = %s", b, pending)
	}
	service := paylinks.NewService(pool)
	for _, id := range []uuid.UUID{flyer.ID, tutoring.ID} {
		link, err := service.Get(context.Background(), student, id)
		if err != nil || link.Status != paylinks.StatusPaid {
			t.Fatalf("link %s = %+v, %v", id, link, err)
		}
	}
	var unattributed int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM incoming_transfers WHERE payment_link_id IS NULL AND status = 'confirmed'`).Scan(&unattributed); err != nil || unattributed != 1 {
		t.Fatalf("unattributed transfers = %d, %v", unattributed, err)
	}
	assertClean(t, pool, w)
}

func TestOldestOpenLinkWithExactAmountIsPaidFirst(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	first := createLink(t, pool, student, "3")
	second := createLink(t, pool, student, "3")
	closed := createLink(t, pool, student, "7")
	service := paylinks.NewService(pool)
	if _, err := service.Close(context.Background(), student, closed.ID); err != nil {
		t.Fatal(err)
	}
	w := newWatcher(pool, simulated.NewProvider(pool))

	firstPayment := send(t, pool, address, "3", 0, "")
	cycle(t, pool, w)
	if got := transferByHash(t, pool, firstPayment.TxHash); got.PaymentLinkID == nil || *got.PaymentLinkID != first.ID {
		t.Fatalf("first payment attributed to %v, want %s", got.PaymentLinkID, first.ID)
	}
	secondPayment := send(t, pool, address, "3", 0, "")
	sevenPayment := send(t, pool, address, "7", 0, "")
	cycle(t, pool, w)
	if got := transferByHash(t, pool, secondPayment.TxHash); got.PaymentLinkID == nil || *got.PaymentLinkID != second.ID {
		t.Fatalf("second payment attributed to %v, want %s", got.PaymentLinkID, second.ID)
	}
	if got := transferByHash(t, pool, sevenPayment.TxHash); got.PaymentLinkID != nil || got.Status != transfers.StatusConfirmed {
		t.Fatalf("payment to a closed link amount must be credited without a link: %+v", got)
	}
	if b, _ := balances(t, pool, student); b.Available != usdt.MustParse("13") {
		t.Fatalf("balances = %+v", b)
	}
}

func TestRevertedTransferIsNeverCredited(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	createLink(t, pool, student, "4")
	w := newWatcher(pool, simulated.NewProvider(pool))

	sent := send(t, pool, address, "4", 0, simulated.OutcomeReverted)
	cycle(t, pool, w)

	got := transferByHash(t, pool, sent.TxHash)
	if got.Status != transfers.StatusFailed || got.FailureReason == nil || *got.FailureReason != "receipt_revert" {
		t.Fatalf("transfer = %+v", got)
	}
	if b, pending := balances(t, pool, student); b.Available != 0 || pending != 0 {
		t.Fatalf("balances = %+v pending = %s", b, pending)
	}
	assertClean(t, pool, w)
}

func TestTransfersAreOnlyCreditedToTheirOwnJar(t *testing.T) {
	pool := testdb.Open(t)
	alice, aliceAddress := testdb.CreateUserWithJar(t, pool)
	bob, _ := testdb.CreateUserWithJar(t, pool)
	w := newWatcher(pool, simulated.NewProvider(pool))

	send(t, pool, aliceAddress, "6", 0, "")
	send(t, pool, tron.SimulatedJarAddress(99999), "50", 0, "")
	cycle(t, pool, w)

	if b, _ := balances(t, pool, alice); b.Available != usdt.MustParse("6") {
		t.Fatalf("alice = %+v", b)
	}
	if b, _ := balances(t, pool, bob); b.Available != 0 {
		t.Fatalf("bob = %+v", b)
	}
}

type fakeProvider struct {
	mu        sync.Mutex
	listings  map[tron.Address]chain.Listing
	outcomes  map[string]chain.Outcome
	balances  map[tron.Address]usdt.Amount
	listErr   error
	listCalls atomic.Int64
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{listings: map[tron.Address]chain.Listing{}, outcomes: map[string]chain.Outcome{}, balances: map[tron.Address]usdt.Amount{}}
}

func (f *fakeProvider) Network() tron.Network { return tron.NetworkSimulated }

func (f *fakeProvider) ListIncoming(ctx context.Context, to tron.Address, since time.Time) (chain.Listing, error) {
	f.listCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return chain.Listing{}, f.listErr
	}
	listing := f.listings[to]
	listing.Complete = true
	return listing, nil
}

func (f *fakeProvider) TransactionOutcome(ctx context.Context, txHash string) (chain.Outcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	outcome, ok := f.outcomes[txHash]
	if !ok {
		return chain.Outcome{State: chain.OutcomeNotFinal}, nil
	}
	return outcome, nil
}

func (f *fakeProvider) TokenBalance(ctx context.Context, address tron.Address) (usdt.Amount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.balances[address], nil
}

func (f *fakeProvider) observe(to tron.Address, hash, amount string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	listing := f.listings[to]
	listing.Observations = append(listing.Observations, chain.Observation{TxHash: hash, From: tron.SimulatedJarAddress(0), To: to, Amount: usdt.MustParse(amount), BlockTime: time.Now()})
	listing.NewestTimestamp = time.Now()
	f.listings[to] = listing
}

func (f *fakeProvider) finalize(hash string, blockTime time.Time, transfers ...chain.TokenTransfer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outcomes[hash] = chain.Outcome{State: chain.OutcomeFinal, BlockNumber: 100, BlockTime: blockTime, Transfers: transfers}
}

func TestFinalAmountComesFromTheSolidifiedLogsNotTheListing(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	createLink(t, pool, student, "4")
	provider := newFakeProvider()
	w := newWatcher(pool, provider)
	hash := testdb.RandomTxHash(t)

	provider.observe(address, hash, "4")
	cycle(t, pool, w)
	provider.finalize(hash, time.Now(),
		chain.TokenTransfer{LogIndex: 0, From: tron.SimulatedJarAddress(0), To: address, Amount: usdt.MustParse("1.5")},
		chain.TokenTransfer{LogIndex: 1, From: tron.SimulatedJarAddress(0), To: tron.SimulatedJarAddress(424242), Amount: usdt.MustParse("100")},
		chain.TokenTransfer{LogIndex: 2, From: tron.SimulatedJarAddress(0), To: address, Amount: usdt.MustParse("1")},
	)
	cycle(t, pool, w)

	got := transferByHash(t, pool, hash)
	if got.Status != transfers.StatusConfirmed || got.Amount != usdt.MustParse("2.5") || got.PaymentLinkID != nil {
		t.Fatalf("transfer = %+v", got)
	}
	if b, _ := balances(t, pool, student); b.Available != usdt.MustParse("2.5") {
		t.Fatalf("balances = %+v", b)
	}
}

func TestSolidifiedTransactionWithoutTransferToJarFails(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	provider := newFakeProvider()
	w := newWatcher(pool, provider)
	hash := testdb.RandomTxHash(t)

	provider.observe(address, hash, "4")
	provider.finalize(hash, time.Now())
	cycle(t, pool, w)

	got := transferByHash(t, pool, hash)
	if got.Status != transfers.StatusFailed || *got.FailureReason != transfers.ReasonNoMatchingTransfer {
		t.Fatalf("transfer = %+v", got)
	}
	if b, _ := balances(t, pool, student); b.Available != 0 {
		t.Fatalf("balances = %+v", b)
	}
}

func TestLinksCreatedAfterThePaymentAreNotAttributed(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	provider := newFakeProvider()
	w := newWatcher(pool, provider)
	hash := testdb.RandomTxHash(t)

	provider.observe(address, hash, "4")
	provider.finalize(hash, time.Now().Add(-time.Hour), chain.TokenTransfer{From: tron.SimulatedJarAddress(0), To: address, Amount: usdt.MustParse("4")})
	link := createLink(t, pool, student, "4")
	cycle(t, pool, w)

	if got := transferByHash(t, pool, hash); got.Status != transfers.StatusConfirmed || got.PaymentLinkID != nil {
		t.Fatalf("transfer = %+v", got)
	}
	open, err := paylinks.NewService(pool).Get(context.Background(), student, link.ID)
	if err != nil || open.Status != paylinks.StatusOpen {
		t.Fatalf("link = %+v, %v", open, err)
	}
}

func TestTransfersThatNeverSolidifyAreDropped(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	provider := newFakeProvider()
	cfg := watcher.DefaultConfig()
	cfg.DropAfter = time.Millisecond
	w := watcher.New(pool, provider, quietLogger, cfg)
	hash := testdb.RandomTxHash(t)

	provider.observe(address, hash, "4")
	makeDue(t, pool)
	if _, err := w.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	makeDue(t, pool)
	if _, err := w.ConfirmOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	got := transferByHash(t, pool, hash)
	if got.Status != transfers.StatusFailed || *got.FailureReason != transfers.ReasonNotSolidified {
		t.Fatalf("transfer = %+v", got)
	}
	if _, pending := balances(t, pool, student); pending != 0 {
		t.Fatalf("pending = %s", pending)
	}
}

func TestNotFinalTransfersBackOff(t *testing.T) {
	pool := testdb.Open(t)
	_, address := testdb.CreateUserWithJar(t, pool)
	provider := newFakeProvider()
	w := newWatcher(pool, provider)
	provider.observe(address, testdb.RandomTxHash(t), "4")
	cycle(t, pool, w)

	var (
		attempts int
		waiting  bool
	)
	err := pool.QueryRow(context.Background(),
		`SELECT check_attempts, next_check_at > now() FROM incoming_transfers`).Scan(&attempts, &waiting)
	if err != nil || attempts != 1 || !waiting {
		t.Fatalf("attempts = %d waiting = %v err = %v", attempts, waiting, err)
	}
	if confirmed, err := w.ConfirmOnce(context.Background()); err != nil || confirmed != 0 {
		t.Fatalf("a rescheduled transfer must not be re-checked early: %d, %v", confirmed, err)
	}
}

func TestConcurrentConfirmationsCreditOnce(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	createLink(t, pool, student, "4")
	provider := newFakeProvider()
	w := newWatcher(pool, provider)
	hash := testdb.RandomTxHash(t)
	provider.observe(address, hash, "4")
	makeDue(t, pool)
	if _, err := w.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	outcome := chain.Outcome{State: chain.OutcomeFinal, BlockNumber: 5, BlockTime: time.Now(),
		Transfers: []chain.TokenTransfer{{From: tron.SimulatedJarAddress(0), To: address, Amount: usdt.MustParse("4")}}}
	id := transferByHash(t, pool, hash).ID

	var (
		wait     sync.WaitGroup
		credited atomic.Int64
	)
	for range 12 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := transfers.Confirm(context.Background(), pool, id, outcome)
			if err != nil {
				t.Error(err)
				return
			}
			if !result.AlreadyDone {
				credited.Add(1)
			}
		}()
	}
	wait.Wait()

	if credited.Load() != 1 {
		t.Fatalf("credited %d times, want 1", credited.Load())
	}
	if b, _ := balances(t, pool, student); b.Available != usdt.MustParse("4") {
		t.Fatalf("balances = %+v", b)
	}
}

func TestParallelWatchersShareWorkWithoutDoubleCrediting(t *testing.T) {
	pool := testdb.Open(t)
	var students []uuid.UUID
	for range 6 {
		student, address := testdb.CreateUserWithJar(t, pool)
		students = append(students, student)
		for _, amount := range []string{"1", "2", "3"} {
			send(t, pool, address, amount, 0, "")
		}
	}
	makeDue(t, pool)

	var wait sync.WaitGroup
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			w := newWatcher(pool, simulated.NewProvider(pool))
			for range 3 {
				if _, err := w.DiscoverOnce(context.Background()); err != nil {
					t.Error(err)
				}
				if _, err := w.ConfirmOnce(context.Background()); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wait.Wait()
	cycle(t, pool, newWatcher(pool, simulated.NewProvider(pool)))

	for _, student := range students {
		if b, pending := balances(t, pool, student); b.Available != usdt.MustParse("6") || pending != 0 {
			t.Fatalf("student %s balances = %+v pending = %s", student, b, pending)
		}
	}
	assertClean(t, pool, newWatcher(pool, simulated.NewProvider(pool)))
}

func TestDiscoveryFailuresAreRecordedAndBackedOff(t *testing.T) {
	pool := testdb.Open(t)
	testdb.CreateUserWithJar(t, pool)
	provider := newFakeProvider()
	provider.listErr = errors.Join(chain.ErrRateLimited, errors.New("slow down"))
	w := newWatcher(pool, provider)

	makeDue(t, pool)
	if _, err := w.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	var (
		failures int
		lastErr  *string
		waiting  bool
	)
	err := pool.QueryRow(context.Background(),
		`SELECT consecutive_failures, last_error, next_poll_at > now() + interval '10 seconds' FROM jar_watch_state`).Scan(&failures, &lastErr, &waiting)
	if err != nil || failures != 1 || lastErr == nil || !waiting {
		t.Fatalf("failures = %d last_error = %v waiting = %v err = %v", failures, lastErr, waiting, err)
	}
	if _, err := w.DiscoverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if provider.listCalls.Load() != 1 {
		t.Fatalf("backed-off jar was polled again: %d calls", provider.listCalls.Load())
	}
}

func TestReconciliationFlagsChainBalanceDifferences(t *testing.T) {
	pool := testdb.Open(t)
	student, address := testdb.CreateUserWithJar(t, pool)
	provider := newFakeProvider()
	w := newWatcher(pool, provider)
	hash := testdb.RandomTxHash(t)
	provider.observe(address, hash, "5")
	provider.finalize(hash, time.Now(), chain.TokenTransfer{From: tron.SimulatedJarAddress(0), To: address, Amount: usdt.MustParse("5")})
	cycle(t, pool, w)

	provider.balances[address] = usdt.MustParse("5")
	assertClean(t, pool, w)

	provider.balances[address] = usdt.MustParse("4")
	if _, err := pool.Exec(context.Background(), `UPDATE jar_watch_state SET last_reconciled_at = NULL`); err != nil {
		t.Fatal(err)
	}
	report, err := w.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Clean() || len(report.ChainBelowLedger) != 1 || report.ChainBelowLedger[0].UserID != student {
		t.Fatalf("report = %+v", report)
	}

	provider.balances[address] = usdt.MustParse("9")
	if _, err := pool.Exec(context.Background(), `UPDATE jar_watch_state SET last_reconciled_at = NULL`); err != nil {
		t.Fatal(err)
	}
	report, err = w.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Clean() || len(report.UnrecordedFunds) != 1 {
		t.Fatalf("report = %+v", report)
	}
}

func TestResolvedTransfersAreImmutable(t *testing.T) {
	pool := testdb.Open(t)
	_, address := testdb.CreateUserWithJar(t, pool)
	w := newWatcher(pool, simulated.NewProvider(pool))
	sent := send(t, pool, address, "4", 0, "")
	cycle(t, pool, w)

	for _, statement := range []string{
		`UPDATE incoming_transfers SET amount = 999000000 WHERE tx_hash = $1`,
		`UPDATE incoming_transfers SET status = 'pending', confirmed_at = NULL WHERE tx_hash = $1`,
		`DELETE FROM incoming_transfers WHERE tx_hash = $1`,
	} {
		if _, err := pool.Exec(context.Background(), statement, sent.TxHash); err == nil {
			t.Errorf("%q must be rejected", statement)
		}
	}
}

func TestWakeBringsAnIdleJarForwardButIsRateLimited(t *testing.T) {
	pool := testdb.Open(t)
	student, _ := testdb.CreateUserWithJar(t, pool)
	ctx := context.Background()
	dueIn := func() time.Duration {
		t.Helper()
		var seconds float64
		if err := pool.QueryRow(ctx, `SELECT extract(epoch FROM next_poll_at - now()) FROM jar_watch_state WHERE user_id = $1`, student).Scan(&seconds); err != nil {
			t.Fatal(err)
		}
		return time.Duration(seconds * float64(time.Second))
	}

	if _, err := pool.Exec(ctx, `UPDATE jar_watch_state SET last_polled_at = now() - interval '1 minute', next_poll_at = now() + interval '5 minutes'`); err != nil {
		t.Fatal(err)
	}
	if err := jars.Wake(ctx, pool, student); err != nil {
		t.Fatal(err)
	}
	if due := dueIn(); due > time.Second {
		t.Fatalf("idle jar must be due now after a wake, due in %s", due)
	}

	if _, err := pool.Exec(ctx, `UPDATE jar_watch_state SET last_polled_at = now(), next_poll_at = now() + interval '5 minutes'`); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := jars.Wake(ctx, pool, student); err != nil {
			t.Fatal(err)
		}
	}
	if due := dueIn(); due < 8*time.Second || due > 11*time.Second {
		t.Fatalf("wakes right after a poll must wait ~10s, due in %s", due)
	}

	if _, err := pool.Exec(ctx, `UPDATE jar_watch_state SET next_poll_at = now() + interval '2 seconds'`); err != nil {
		t.Fatal(err)
	}
	if err := jars.Wake(ctx, pool, student); err != nil {
		t.Fatal(err)
	}
	if due := dueIn(); due > 3*time.Second {
		t.Fatalf("a wake must never push a poll later, due in %s", due)
	}
}
