package watcher

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/ledger"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

type TransferMismatch struct {
	TransferID     uuid.UUID
	TransferAmount usdt.Amount
	CreditedAmount usdt.Amount
}

type BalanceMismatch struct {
	UserID       uuid.UUID
	Address      tron.Address
	ChainBalance usdt.Amount
	LedgerHeld   usdt.Amount
	Pending      usdt.Amount
}

type Report struct {
	LedgerDiscrepancies []ledger.Discrepancy
	TransferMismatches  []TransferMismatch
	ChainBelowLedger    []BalanceMismatch
	UnrecordedFunds     []BalanceMismatch
	JarsChecked         int
	ChainErrors         int
}

func (r Report) Clean() bool {
	return len(r.LedgerDiscrepancies) == 0 && len(r.TransferMismatches) == 0 && len(r.ChainBelowLedger) == 0
}

func (r Report) Log(logger *slog.Logger) {
	for _, d := range r.LedgerDiscrepancies {
		logger.Error("ledger cache disagrees with postings", "account_id", d.AccountID, "kind", d.Kind,
			"cached", d.CachedBalance.String(), "posted", d.PostedBalance.String())
	}
	for _, m := range r.TransferMismatches {
		logger.Error("confirmed transfer and ledger deposit disagree", "transfer_id", m.TransferID,
			"transfer_amount", m.TransferAmount.String(), "credited_amount", m.CreditedAmount.String())
	}
	for _, m := range r.ChainBelowLedger {
		logger.Error("jar holds less on chain than the ledger says", "user_id", m.UserID, "address", m.Address,
			"chain", m.ChainBalance.String(), "ledger", m.LedgerHeld.String())
	}
	for _, m := range r.UnrecordedFunds {
		logger.Warn("jar holds funds on chain that are not in the ledger", "user_id", m.UserID, "address", m.Address,
			"chain", m.ChainBalance.String(), "ledger", m.LedgerHeld.String(), "pending", m.Pending.String())
	}
	logger.Info("reconciliation finished", "clean", r.Clean(), "jars_checked", r.JarsChecked, "chain_errors", r.ChainErrors)
}

func (w *Watcher) ReconcileOnce(ctx context.Context) (Report, error) {
	var report Report
	discrepancies, err := ledger.Reconcile(ctx, w.pool)
	if err != nil {
		return report, err
	}
	report.LedgerDiscrepancies = discrepancies

	rows, err := w.pool.Query(ctx, `
		SELECT t.id, t.amount, coalesce(sum(p.amount) FILTER (WHERE p.amount > 0), 0)::bigint
		FROM incoming_transfers t
		LEFT JOIN journal_entries je ON je.incoming_transfer_id = t.id AND je.kind = 'deposit'
		LEFT JOIN postings p ON p.journal_entry_id = je.id
		WHERE t.status = 'confirmed' OR je.id IS NOT NULL
		GROUP BY t.id
		HAVING t.status <> 'confirmed' OR t.amount <> coalesce(sum(p.amount) FILTER (WHERE p.amount > 0), 0)`)
	if err != nil {
		return report, fmt.Errorf("compare transfers with deposits: %w", err)
	}
	report.TransferMismatches, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TransferMismatch, error) {
		var (
			mismatch         TransferMismatch
			transfer, credit int64
		)
		err := row.Scan(&mismatch.TransferID, &transfer, &credit)
		mismatch.TransferAmount, mismatch.CreditedAmount = usdt.Amount(transfer), usdt.Amount(credit)
		return mismatch, err
	})
	if err != nil {
		return report, fmt.Errorf("read transfer mismatches: %w", err)
	}

	jarRows, err := w.pool.Query(ctx, `
		UPDATE jar_watch_state s SET last_reconciled_at = now()
		FROM (
			SELECT user_id FROM jar_watch_state
			WHERE network = $1
			ORDER BY last_reconciled_at NULLS FIRST
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		) due
		WHERE s.user_id = due.user_id
		RETURNING s.user_id, s.address`,
		w.network, w.cfg.ReconcileBatch)
	if err != nil {
		return report, fmt.Errorf("claim jars to reconcile: %w", err)
	}
	type jarRef struct {
		userID  uuid.UUID
		address tron.Address
	}
	jars, err := pgx.CollectRows(jarRows, func(row pgx.CollectableRow) (jarRef, error) {
		var jar jarRef
		err := row.Scan(&jar.userID, &jar.address)
		return jar, err
	})
	if err != nil {
		return report, fmt.Errorf("read jars to reconcile: %w", err)
	}

	for _, jar := range jars {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		chainBalance, err := w.provider.TokenBalance(ctx, jar.address)
		if err != nil {
			report.ChainErrors++
			w.logger.Warn("reading jar balance on chain failed", "address", jar.address, "error", err)
			continue
		}
		var held, pending int64
		err = w.pool.QueryRow(ctx, `
			SELECT
				(SELECT coalesce(sum(balance), 0) FROM ledger_accounts WHERE user_id = $1 AND kind IN ('jar_available', 'jar_reserved'))::bigint,
				(SELECT coalesce(sum(amount), 0) FROM incoming_transfers WHERE user_id = $1 AND status = 'pending')::bigint`,
			jar.userID,
		).Scan(&held, &pending)
		if err != nil {
			return report, fmt.Errorf("read ledger holdings: %w", err)
		}
		report.JarsChecked++
		mismatch := BalanceMismatch{UserID: jar.userID, Address: jar.address, ChainBalance: chainBalance, LedgerHeld: usdt.Amount(held), Pending: usdt.Amount(pending)}
		switch {
		case chainBalance < mismatch.LedgerHeld:
			report.ChainBelowLedger = append(report.ChainBelowLedger, mismatch)
		case chainBalance > mismatch.LedgerHeld+mismatch.Pending:
			report.UnrecordedFunds = append(report.UnrecordedFunds, mismatch)
		}
	}
	return report, nil
}
