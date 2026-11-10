package transfers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain"
	"gigjar/internal/database"
	"gigjar/internal/ledger"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusFailed    Status = "failed"
)

const (
	ReasonNotSolidified      = "not_solidified"
	ReasonNoMatchingTransfer = "no_matching_transfer"
)

var ErrInvalidCursor = errors.New("invalid cursor")

type Transfer struct {
	ID            uuid.UUID
	Network       tron.Network
	TxHash        string
	UserID        uuid.UUID
	To            tron.Address
	From          tron.Address
	Amount        usdt.Amount
	Status        Status
	FailureReason *string
	BlockNumber   *int64
	BlockTime     *time.Time
	PaymentLinkID *uuid.UUID
	ObservedAt    time.Time
	ConfirmedAt   *time.Time
	FailedAt      *time.Time
}

const transferColumns = `id, network, tx_hash, user_id, to_address, from_address, amount, status, failure_reason,
	block_number, block_time, payment_link_id, observed_at, confirmed_at, failed_at`

func scanTransfer(row pgx.Row) (Transfer, error) {
	var (
		transfer Transfer
		amount   int64
	)
	err := row.Scan(&transfer.ID, &transfer.Network, &transfer.TxHash, &transfer.UserID, &transfer.To, &transfer.From,
		&amount, &transfer.Status, &transfer.FailureReason, &transfer.BlockNumber, &transfer.BlockTime,
		&transfer.PaymentLinkID, &transfer.ObservedAt, &transfer.ConfirmedAt, &transfer.FailedAt)
	transfer.Amount = usdt.Amount(amount)
	return transfer, err
}

func RecordObservations(ctx context.Context, db database.Querier, network tron.Network, userID uuid.UUID, observations []chain.Observation) (int, error) {
	inserted := 0
	for _, observation := range observations {
		if !observation.Amount.IsPositive() {
			continue
		}
		tag, err := db.Exec(ctx, `
			INSERT INTO incoming_transfers (network, tx_hash, user_id, to_address, from_address, amount)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (network, tx_hash, to_address) DO NOTHING`,
			network, observation.TxHash, userID, observation.To, observation.From, int64(observation.Amount))
		if err != nil {
			return inserted, fmt.Errorf("record observation %s: %w", observation.TxHash, err)
		}
		inserted += int(tag.RowsAffected())
	}
	return inserted, nil
}

type Due struct {
	ID         uuid.UUID
	TxHash     string
	UserID     uuid.UUID
	To         tron.Address
	ObservedAt time.Time
	Attempts   int
}

func ClaimDue(ctx context.Context, db database.Querier, network tron.Network, limit int, lease time.Duration) ([]Due, error) {
	rows, err := db.Query(ctx, `
		UPDATE incoming_transfers t
		SET next_check_at = now() + make_interval(secs => $3), check_attempts = t.check_attempts + 1
		FROM (
			SELECT id FROM incoming_transfers
			WHERE network = $1 AND status = 'pending' AND next_check_at <= now()
			ORDER BY next_check_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		) due
		WHERE t.id = due.id
		RETURNING t.id, t.tx_hash, t.user_id, t.to_address, t.observed_at, t.check_attempts`,
		network, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("claim pending transfers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Due, error) {
		var due Due
		err := row.Scan(&due.ID, &due.TxHash, &due.UserID, &due.To, &due.ObservedAt, &due.Attempts)
		return due, err
	})
}

func Reschedule(ctx context.Context, db database.Querier, id uuid.UUID, delay time.Duration) error {
	_, err := db.Exec(ctx, `
		UPDATE incoming_transfers SET next_check_at = now() + make_interval(secs => $2)
		WHERE id = $1 AND status = 'pending'`,
		id, delay.Seconds())
	if err != nil {
		return fmt.Errorf("reschedule transfer check: %w", err)
	}
	return nil
}

func MarkFailed(ctx context.Context, db database.Querier, id uuid.UUID, reason string) error {
	_, err := db.Exec(ctx, `
		UPDATE incoming_transfers SET status = 'failed', failure_reason = $2, failed_at = now()
		WHERE id = $1 AND status = 'pending'`,
		id, reason)
	if err != nil {
		return fmt.Errorf("mark transfer failed: %w", err)
	}
	return nil
}

type Confirmation struct {
	Status        Status
	Amount        usdt.Amount
	PaymentLinkID *uuid.UUID
	EntryID       uuid.UUID
	AlreadyDone   bool
}

func Confirm(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, outcome chain.Outcome) (Confirmation, error) {
	if outcome.State != chain.OutcomeFinal {
		return Confirmation{}, fmt.Errorf("confirm requires a final outcome, got %s", outcome.State)
	}
	var result Confirmation
	err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
		var (
			userID uuid.UUID
			to     tron.Address
			status Status
		)
		err := tx.QueryRow(ctx,
			`SELECT user_id, to_address, status FROM incoming_transfers WHERE id = $1 FOR UPDATE`, id,
		).Scan(&userID, &to, &status)
		if err != nil {
			return fmt.Errorf("lock transfer: %w", err)
		}
		if status != StatusPending {
			result = Confirmation{Status: status, AlreadyDone: true}
			return nil
		}

		matched := outcome.TransfersTo(to)
		if len(matched) == 0 {
			_, err := tx.Exec(ctx, `
				UPDATE incoming_transfers SET status = 'failed', failure_reason = $2, failed_at = now() WHERE id = $1`,
				id, ReasonNoMatchingTransfer)
			result = Confirmation{Status: StatusFailed}
			return err
		}

		var (
			total      usdt.Amount
			logIndexes []int32
		)
		for _, transfer := range matched {
			total, err = usdt.Add(total, transfer.Amount)
			if err != nil {
				return err
			}
			logIndexes = append(logIndexes, int32(transfer.LogIndex))
		}

		linkID, err := attributeToLink(ctx, tx, userID, total, outcome.BlockTime)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			UPDATE incoming_transfers
			SET status = 'confirmed', amount = $2, from_address = $3, log_indexes = $4,
			    block_number = $5, block_time = $6, payment_link_id = $7, confirmed_at = now()
			WHERE id = $1`,
			id, int64(total), matched[0].From, logIndexes, outcome.BlockNumber, outcome.BlockTime, linkID)
		if err != nil {
			return fmt.Errorf("confirm transfer: %w", err)
		}

		recorded, err := ledger.RecordDeposit(ctx, tx, userID, id, total)
		if err != nil {
			return err
		}
		result = Confirmation{Status: StatusConfirmed, Amount: total, PaymentLinkID: linkID, EntryID: recorded.EntryID}
		return nil
	})
	if err != nil {
		return Confirmation{}, err
	}
	return result, nil
}

func attributeToLink(ctx context.Context, tx pgx.Tx, userID uuid.UUID, amount usdt.Amount, blockTime time.Time) (*uuid.UUID, error) {
	var linkID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM payment_links
		WHERE user_id = $1 AND status = 'open' AND amount = $2 AND created_at <= $3
		ORDER BY created_at, id
		LIMIT 1
		FOR UPDATE`,
		userID, int64(amount), blockTime,
	).Scan(&linkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find payment link for transfer: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE payment_links SET status = 'paid', paid_at = now() WHERE id = $1`, linkID); err != nil {
		return nil, fmt.Errorf("mark payment link paid: %w", err)
	}
	return &linkID, nil
}

func PendingTotal(ctx context.Context, db database.Querier, userID uuid.UUID) (usdt.Amount, error) {
	var total int64
	err := db.QueryRow(ctx, `
		SELECT coalesce(sum(amount), 0)::bigint FROM incoming_transfers WHERE user_id = $1 AND status = 'pending'`,
		userID,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("pending total: %w", err)
	}
	return usdt.Amount(total), nil
}

type Page struct {
	Transfers  []Transfer
	NextCursor string
}

func List(ctx context.Context, db database.Querier, userID uuid.UUID, cursor string, limit int) (Page, error) {
	limit = clampLimit(limit)
	observedAt, cursorID, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	rows, err := db.Query(ctx, `
		SELECT `+transferColumns+` FROM incoming_transfers
		WHERE user_id = $1 AND ($2::timestamptz IS NULL OR (observed_at, id) < ($2, $3))
		ORDER BY observed_at DESC, id DESC
		LIMIT $4`,
		userID, observedAt, cursorID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list incoming transfers: %w", err)
	}
	transfers, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Transfer, error) { return scanTransfer(row) })
	if err != nil {
		return Page{}, fmt.Errorf("read incoming transfers: %w", err)
	}
	page := Page{Transfers: transfers}
	if len(transfers) > limit {
		page.Transfers = transfers[:limit]
		last := page.Transfers[limit-1]
		page.NextCursor = encodeCursor(last.ObservedAt, last.ID)
	}
	return page, nil
}

func ForLink(ctx context.Context, db database.Querier, userID, linkID uuid.UUID) ([]Transfer, error) {
	rows, err := db.Query(ctx, `
		SELECT `+transferColumns+` FROM incoming_transfers
		WHERE user_id = $1 AND payment_link_id = $2
		ORDER BY observed_at, id`,
		userID, linkID)
	if err != nil {
		return nil, fmt.Errorf("list link transfers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Transfer, error) { return scanTransfer(row) })
}

func Get(ctx context.Context, db database.Querier, id uuid.UUID) (Transfer, error) {
	return scanTransfer(db.QueryRow(ctx, `SELECT `+transferColumns+` FROM incoming_transfers WHERE id = $1`, id))
}

func encodeCursor(observedAt time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(observedAt.UnixMicro(), 10) + "|" + id.String()))
}

func decodeCursor(cursor string) (*time.Time, uuid.UUID, error) {
	if cursor == "" {
		return nil, uuid.Nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	micros, idText, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	value, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	observedAt := time.UnixMicro(value)
	return &observedAt, id, nil
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 25
	case limit > 100:
		return 100
	default:
		return limit
	}
}
