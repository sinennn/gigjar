//go:build integration

package ledger_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/database"
	"gigjar/internal/ledger"
	"gigjar/internal/testdb"
	"gigjar/internal/usdt"
)

func inTx(t *testing.T, pool *pgxpool.Pool, work func(tx pgx.Tx) error) error {
	t.Helper()
	return database.WithTx(context.Background(), pool, work)
}

func balancesOf(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) ledger.Balances {
	t.Helper()
	balances, err := ledger.UserBalances(context.Background(), pool, userID)
	if err != nil {
		t.Fatal(err)
	}
	return balances
}

func deposit(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID, amount string) uuid.UUID {
	t.Helper()
	transferID := testdb.IncomingTransfer(t, pool, userID)
	err := inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.RecordDeposit(context.Background(), tx, userID, transferID, usdt.MustParse(amount))
		return err
	})
	if err != nil {
		t.Fatalf("deposit %s: %v", amount, err)
	}
	return transferID
}

func assertReconciled(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	discrepancies, err := ledger.Reconcile(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(discrepancies) != 0 {
		t.Fatalf("ledger discrepancies: %+v", discrepancies)
	}
}

func TestPitchDeckScenario(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	student := testdb.CreateUser(t, pool)

	deposit(t, pool, student, "4")
	deposit(t, pool, student, "3")
	deposit(t, pool, student, "5")

	if got := balancesOf(t, pool, student); got.Available != usdt.MustParse("12") {
		t.Fatalf("available = %s, want 12", got.Available)
	}

	settlementID := uuid.New()
	amount := usdt.MustParse("12")
	if err := inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.ReserveForSettlement(ctx, tx, student, settlementID, amount)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := balancesOf(t, pool, student); got.Available != 0 || got.Reserved != amount {
		t.Fatalf("after reserve: %+v", got)
	}

	if err := inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.CompleteSettlement(ctx, tx, student, settlementID, amount)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := balancesOf(t, pool, student); got != (ledger.Balances{Settled: amount}) {
		t.Fatalf("after completion: %+v", got)
	}

	movements, err := ledger.ListMovements(ctx, pool, student, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds := []ledger.EntryKind{ledger.EntrySettlementCompleted, ledger.EntrySettlementReserved, ledger.EntryDeposit, ledger.EntryDeposit, ledger.EntryDeposit}
	if len(movements) != len(wantKinds) {
		t.Fatalf("got %d movements, want %d", len(movements), len(wantKinds))
	}
	for index, kind := range wantKinds {
		if movements[index].Kind != kind {
			t.Errorf("movement %d kind = %s, want %s", index, movements[index].Kind, kind)
		}
	}
	if movements[4].AvailableDelta != usdt.MustParse("4") {
		t.Errorf("first deposit delta = %s", movements[4].AvailableDelta)
	}
	if movements[1].AvailableDelta != -amount || movements[1].ReservedDelta != amount {
		t.Errorf("reserve deltas = %+v", movements[1])
	}

	page, err := ledger.ListMovements(ctx, pool, student, movements[1].Sequence, 10)
	if err != nil || len(page) != 3 {
		t.Fatalf("cursor page = %d movements, err %v", len(page), err)
	}
	assertReconciled(t, pool)
}

func TestDepositIsIdempotent(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	student := testdb.CreateUser(t, pool)
	transferID := testdb.IncomingTransfer(t, pool, student)

	for attempt := 0; attempt < 3; attempt++ {
		var recorded ledger.Recorded
		err := inTx(t, pool, func(tx pgx.Tx) error {
			var err error
			recorded, err = ledger.RecordDeposit(ctx, tx, student, transferID, usdt.MustParse("4"))
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if recorded.AlreadyRecorded != (attempt > 0) {
			t.Errorf("attempt %d already recorded = %v", attempt, recorded.AlreadyRecorded)
		}
	}
	if got := balancesOf(t, pool, student); got.Available != usdt.MustParse("4") {
		t.Fatalf("available = %s after duplicate deposits", got.Available)
	}

	err := inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.RecordDeposit(ctx, tx, student, transferID, usdt.MustParse("5"))
		return err
	})
	if !errors.Is(err, ledger.ErrIdempotencyKeyConflict) {
		t.Fatalf("conflicting replay error = %v", err)
	}
}

func TestReserveCannotOverspend(t *testing.T) {
	pool := testdb.Open(t)
	student := testdb.CreateUser(t, pool)
	deposit(t, pool, student, "10")

	err := inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.ReserveForSettlement(context.Background(), tx, student, uuid.New(), usdt.MustParse("10.000001"))
		return err
	})
	if !errors.Is(err, ledger.ErrInsufficientFunds) {
		t.Fatalf("error = %v, want ErrInsufficientFunds", err)
	}
	if got := balancesOf(t, pool, student); got.Available != usdt.MustParse("10") || got.Reserved != 0 {
		t.Fatalf("balances changed after rejected reserve: %+v", got)
	}
	assertReconciled(t, pool)
}

func TestReleaseReturnsFundsToJar(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	student := testdb.CreateUser(t, pool)
	deposit(t, pool, student, "15")
	settlementID := uuid.New()
	amount := usdt.MustParse("15")

	err := inTx(t, pool, func(tx pgx.Tx) error {
		if _, err := ledger.ReserveForSettlement(ctx, tx, student, settlementID, amount); err != nil {
			return err
		}
		_, err := ledger.ReleaseSettlement(ctx, tx, student, settlementID, amount)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := balancesOf(t, pool, student); got != (ledger.Balances{Available: amount}) {
		t.Fatalf("after release: %+v", got)
	}

	err = inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.CompleteSettlement(ctx, tx, student, settlementID, amount)
		return err
	})
	if !errors.Is(err, ledger.ErrInsufficientFunds) {
		t.Fatalf("completing a released settlement must fail, got %v", err)
	}
}

func TestConcurrentReservesNeverDoubleSpend(t *testing.T) {
	pool := testdb.Open(t)
	student := testdb.CreateUser(t, pool)
	deposit(t, pool, student, "12")

	const workers = 16
	var (
		wait      sync.WaitGroup
		succeeded atomic.Int32
		start     = make(chan struct{})
	)
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			err := inTx(t, pool, func(tx pgx.Tx) error {
				_, err := ledger.ReserveForSettlement(context.Background(), tx, student, uuid.New(), usdt.MustParse("12"))
				return err
			})
			switch {
			case err == nil:
				succeeded.Add(1)
			case errors.Is(err, ledger.ErrInsufficientFunds):
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wait.Wait()

	if succeeded.Load() != 1 {
		t.Fatalf("%d reserves succeeded, want exactly 1", succeeded.Load())
	}
	if got := balancesOf(t, pool, student); got.Available != 0 || got.Reserved != usdt.MustParse("12") {
		t.Fatalf("balances = %+v", got)
	}
	assertReconciled(t, pool)
}

func TestConcurrentDepositsAndReservesStayConsistent(t *testing.T) {
	pool := testdb.Open(t)
	students := []uuid.UUID{testdb.CreateUser(t, pool), testdb.CreateUser(t, pool), testdb.CreateUser(t, pool)}

	var wait sync.WaitGroup
	for _, student := range students {
		for range 10 {
			wait.Add(2)
			go func() {
				defer wait.Done()
				deposit(t, pool, student, "1.5")
			}()
			go func() {
				defer wait.Done()
				err := inTx(t, pool, func(tx pgx.Tx) error {
					_, err := ledger.ReserveForSettlement(context.Background(), tx, student, uuid.New(), usdt.MustParse("1"))
					return err
				})
				if err != nil && !errors.Is(err, ledger.ErrInsufficientFunds) {
					t.Errorf("reserve: %v", err)
				}
			}()
		}
	}
	wait.Wait()

	for _, student := range students {
		got := balancesOf(t, pool, student)
		total, _ := usdt.Add(got.Available, got.Reserved)
		if total != usdt.MustParse("15") {
			t.Errorf("student %s holds %s across available and reserved, want 15", student, total)
		}
	}
	assertReconciled(t, pool)
}

func TestLedgerRowsAreImmutable(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	student := testdb.CreateUser(t, pool)
	deposit(t, pool, student, "4")

	for _, statement := range []string{
		`UPDATE postings SET amount = amount * 2`,
		`DELETE FROM postings`,
		`UPDATE journal_entries SET kind = 'deposit'`,
		`DELETE FROM journal_entries`,
	} {
		if _, err := pool.Exec(ctx, statement); err == nil {
			t.Errorf("%q succeeded", statement)
		}
	}
	assertReconciled(t, pool)
}

func TestUnbalancedEntriesAreRejected(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	student := testdb.CreateUser(t, pool)
	transferID := testdb.IncomingTransfer(t, pool, student)

	err := inTx(t, pool, func(tx pgx.Tx) error {
		var entryID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO journal_entries (kind, idempotency_key, user_id, incoming_transfer_id)
			VALUES ('deposit', 'forged', $1, $2) RETURNING id`, student, transferID).Scan(&entryID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO postings (journal_entry_id, account_id, amount)
			SELECT $1, id, 1000000 FROM ledger_accounts WHERE user_id = $2 AND kind = 'jar_available'`,
			entryID, student)
		return err
	})
	if !database.IsCheckViolation(err, "journal_entries_balanced") {
		t.Fatalf("expected unbalanced entry rejection, got %v", err)
	}
	if got := balancesOf(t, pool, student); got.Available != 0 {
		t.Fatalf("forged credit persisted: %+v", got)
	}
}

func TestDepositsMustReferenceARealIncomingTransfer(t *testing.T) {
	pool := testdb.Open(t)
	student := testdb.CreateUser(t, pool)
	err := inTx(t, pool, func(tx pgx.Tx) error {
		_, err := ledger.RecordDeposit(context.Background(), tx, student, uuid.New(), usdt.MustParse("4"))
		return err
	})
	if err == nil {
		t.Fatal("a deposit without an incoming transfer must be rejected")
	}
	if got := balancesOf(t, pool, student); got.Available != 0 {
		t.Fatalf("balances = %+v", got)
	}
}
