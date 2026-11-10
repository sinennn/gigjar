package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/database"
	"gigjar/internal/usdt"
)

type Balances struct {
	Available usdt.Amount
	Reserved  usdt.Amount
	Settled   usdt.Amount
}

type Movement struct {
	EntryID            uuid.UUID
	Sequence           int64
	Kind               EntryKind
	AvailableDelta     usdt.Amount
	ReservedDelta      usdt.Amount
	SettledDelta       usdt.Amount
	IncomingTransferID *uuid.UUID
	SettlementID       *uuid.UUID
	CreatedAt          time.Time
}

type Discrepancy struct {
	AccountID     int64
	UserID        *uuid.UUID
	Kind          AccountKind
	CachedBalance usdt.Amount
	PostedBalance usdt.Amount
}

func UserBalances(ctx context.Context, db database.Querier, userID uuid.UUID) (Balances, error) {
	var available, reserved, settled int64
	err := db.QueryRow(ctx, `
		SELECT
			coalesce(sum(balance) FILTER (WHERE kind = 'jar_available'), 0),
			coalesce(sum(balance) FILTER (WHERE kind = 'jar_reserved'), 0),
			coalesce(sum(balance) FILTER (WHERE kind = 'settled'), 0)
		FROM ledger_accounts
		WHERE user_id = $1`,
		userID,
	).Scan(&available, &reserved, &settled)
	if err != nil {
		return Balances{}, fmt.Errorf("read balances: %w", err)
	}
	return Balances{
		Available: usdt.Amount(available),
		Reserved:  usdt.Amount(reserved),
		Settled:   usdt.Amount(settled),
	}, nil
}

func ListMovements(ctx context.Context, db database.Querier, userID uuid.UUID, beforeSequence int64, limit int) ([]Movement, error) {
	rows, err := db.Query(ctx, `
		SELECT
			je.id,
			je.sequence,
			je.kind,
			coalesce(sum(p.amount) FILTER (WHERE a.kind = 'jar_available'), 0),
			coalesce(sum(p.amount) FILTER (WHERE a.kind = 'jar_reserved'), 0),
			coalesce(sum(p.amount) FILTER (WHERE a.kind = 'settled'), 0),
			je.incoming_transfer_id,
			je.settlement_id,
			je.created_at
		FROM journal_entries je
		JOIN postings p ON p.journal_entry_id = je.id
		JOIN ledger_accounts a ON a.id = p.account_id
		WHERE je.user_id = $1 AND ($2 = 0 OR je.sequence < $2)
		GROUP BY je.id
		ORDER BY je.sequence DESC
		LIMIT $3`,
		userID, beforeSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list movements: %w", err)
	}
	movements, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Movement, error) {
		var (
			movement                    Movement
			available, reserved, settle int64
		)
		err := row.Scan(&movement.EntryID, &movement.Sequence, &movement.Kind,
			&available, &reserved, &settle,
			&movement.IncomingTransferID, &movement.SettlementID, &movement.CreatedAt)
		movement.AvailableDelta = usdt.Amount(available)
		movement.ReservedDelta = usdt.Amount(reserved)
		movement.SettledDelta = usdt.Amount(settle)
		return movement, err
	})
	if err != nil {
		return nil, fmt.Errorf("read movements: %w", err)
	}
	return movements, nil
}

func Reconcile(ctx context.Context, db database.Querier) ([]Discrepancy, error) {
	rows, err := db.Query(ctx, `
		SELECT a.id, a.user_id, a.kind, a.balance, coalesce(sum(p.amount), 0)
		FROM ledger_accounts a
		LEFT JOIN postings p ON p.account_id = a.id
		GROUP BY a.id
		HAVING a.balance <> coalesce(sum(p.amount), 0)
		ORDER BY a.id`)
	if err != nil {
		return nil, fmt.Errorf("reconcile ledger: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Discrepancy, error) {
		var (
			discrepancy    Discrepancy
			cached, posted int64
		)
		err := row.Scan(&discrepancy.AccountID, &discrepancy.UserID, &discrepancy.Kind, &cached, &posted)
		discrepancy.CachedBalance = usdt.Amount(cached)
		discrepancy.PostedBalance = usdt.Amount(posted)
		return discrepancy, err
	})
}
