package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/database"
	"gigjar/internal/usdt"
)

type AccountKind string

const (
	AccountJarAvailable AccountKind = "jar_available"
	AccountJarReserved  AccountKind = "jar_reserved"
	AccountSettled      AccountKind = "settled"
	AccountChainInflow  AccountKind = "chain_inflow"
)

var userAccountKinds = []AccountKind{AccountJarAvailable, AccountJarReserved, AccountSettled}

type EntryKind string

const (
	EntryDeposit             EntryKind = "deposit"
	EntrySettlementReserved  EntryKind = "settlement_reserved"
	EntrySettlementCompleted EntryKind = "settlement_completed"
	EntrySettlementReleased  EntryKind = "settlement_released"
)

var (
	ErrInsufficientFunds      = errors.New("insufficient funds")
	ErrNonPositiveAmount      = errors.New("ledger amounts must be positive")
	ErrIdempotencyKeyConflict = errors.New("idempotency key already used for a different movement")
	ErrAccountsMissing        = errors.New("ledger accounts are missing for user")
)

type Recorded struct {
	EntryID         uuid.UUID
	AlreadyRecorded bool
}

type entry struct {
	kind               EntryKind
	idempotencyKey     string
	userID             uuid.UUID
	incomingTransferID *uuid.UUID
	settlementID       *uuid.UUID
	amount             usdt.Amount
	debit              AccountKind
	credit             AccountKind
}

func OpenUserAccounts(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	for _, kind := range userAccountKinds {
		_, err := tx.Exec(ctx, `
			INSERT INTO ledger_accounts (user_id, kind) VALUES ($1, $2)
			ON CONFLICT (user_id, kind) WHERE user_id IS NOT NULL DO NOTHING`,
			userID, kind)
		if err != nil {
			return fmt.Errorf("open %s account: %w", kind, err)
		}
	}
	return nil
}

func RecordDeposit(ctx context.Context, tx pgx.Tx, userID, incomingTransferID uuid.UUID, amount usdt.Amount) (Recorded, error) {
	return record(ctx, tx, entry{
		kind:               EntryDeposit,
		idempotencyKey:     "deposit:" + incomingTransferID.String(),
		userID:             userID,
		incomingTransferID: &incomingTransferID,
		amount:             amount,
		debit:              AccountChainInflow,
		credit:             AccountJarAvailable,
	})
}

func ReserveForSettlement(ctx context.Context, tx pgx.Tx, userID, settlementID uuid.UUID, amount usdt.Amount) (Recorded, error) {
	return record(ctx, tx, entry{
		kind:           EntrySettlementReserved,
		idempotencyKey: "settlement-reserve:" + settlementID.String(),
		userID:         userID,
		settlementID:   &settlementID,
		amount:         amount,
		debit:          AccountJarAvailable,
		credit:         AccountJarReserved,
	})
}

func CompleteSettlement(ctx context.Context, tx pgx.Tx, userID, settlementID uuid.UUID, amount usdt.Amount) (Recorded, error) {
	return record(ctx, tx, entry{
		kind:           EntrySettlementCompleted,
		idempotencyKey: "settlement-complete:" + settlementID.String(),
		userID:         userID,
		settlementID:   &settlementID,
		amount:         amount,
		debit:          AccountJarReserved,
		credit:         AccountSettled,
	})
}

func ReleaseSettlement(ctx context.Context, tx pgx.Tx, userID, settlementID uuid.UUID, amount usdt.Amount) (Recorded, error) {
	return record(ctx, tx, entry{
		kind:           EntrySettlementReleased,
		idempotencyKey: "settlement-release:" + settlementID.String(),
		userID:         userID,
		settlementID:   &settlementID,
		amount:         amount,
		debit:          AccountJarReserved,
		credit:         AccountJarAvailable,
	})
}

type lockedAccount struct {
	id     int64
	kind   AccountKind
	amount usdt.Amount
}

func record(ctx context.Context, tx pgx.Tx, e entry) (Recorded, error) {
	if !e.amount.IsPositive() {
		return Recorded{}, ErrNonPositiveAmount
	}

	accounts, err := lockAccounts(ctx, tx, e)
	if err != nil {
		return Recorded{}, err
	}

	var entryID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO journal_entries (kind, idempotency_key, user_id, incoming_transfer_id, settlement_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`,
		e.kind, e.idempotencyKey, e.userID, e.incomingTransferID, e.settlementID,
	).Scan(&entryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return existingEntry(ctx, tx, e)
	}
	if err != nil {
		return Recorded{}, fmt.Errorf("insert journal entry: %w", err)
	}

	for _, account := range accounts {
		_, err := tx.Exec(ctx,
			`INSERT INTO postings (journal_entry_id, account_id, amount) VALUES ($1, $2, $3)`,
			entryID, account.id, int64(account.amount))
		if database.IsCheckViolation(err, "ledger_accounts_user_balance_non_negative") {
			return Recorded{}, ErrInsufficientFunds
		}
		if err != nil {
			return Recorded{}, fmt.Errorf("insert posting for %s: %w", account.kind, err)
		}
	}
	return Recorded{EntryID: entryID}, nil
}

func lockAccounts(ctx context.Context, tx pgx.Tx, e entry) ([]lockedAccount, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, kind FROM ledger_accounts
		WHERE (user_id = $1 AND kind = ANY($2)) OR (user_id IS NULL AND kind = ANY($2))
		ORDER BY id
		FOR UPDATE`,
		e.userID, []string{string(e.debit), string(e.credit)})
	if err != nil {
		return nil, fmt.Errorf("lock ledger accounts: %w", err)
	}
	accounts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (lockedAccount, error) {
		var account lockedAccount
		err := row.Scan(&account.id, &account.kind)
		return account, err
	})
	if err != nil {
		return nil, fmt.Errorf("read ledger accounts: %w", err)
	}
	if len(accounts) != 2 {
		return nil, fmt.Errorf("%w: %s", ErrAccountsMissing, e.userID)
	}
	for index := range accounts {
		switch accounts[index].kind {
		case e.debit:
			accounts[index].amount = -e.amount
		case e.credit:
			accounts[index].amount = e.amount
		}
	}
	return accounts, nil
}

func existingEntry(ctx context.Context, tx pgx.Tx, e entry) (Recorded, error) {
	var (
		entryID uuid.UUID
		kind    EntryKind
		userID  uuid.UUID
		credit  int64
	)
	err := tx.QueryRow(ctx, `
		SELECT je.id, je.kind, je.user_id, coalesce(sum(p.amount) FILTER (WHERE p.amount > 0), 0)
		FROM journal_entries je
		LEFT JOIN postings p ON p.journal_entry_id = je.id
		WHERE je.idempotency_key = $1
		GROUP BY je.id`,
		e.idempotencyKey,
	).Scan(&entryID, &kind, &userID, &credit)
	if err != nil {
		return Recorded{}, fmt.Errorf("load existing journal entry: %w", err)
	}
	if kind != e.kind || userID != e.userID || usdt.Amount(credit) != e.amount {
		return Recorded{}, fmt.Errorf("%w: %s", ErrIdempotencyKeyConflict, e.idempotencyKey)
	}
	return Recorded{EntryID: entryID, AlreadyRecorded: true}, nil
}
