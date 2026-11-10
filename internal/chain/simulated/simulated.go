package simulated

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"gigjar/internal/chain"
	"gigjar/internal/database"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

const (
	OutcomeSuccess  = "success"
	OutcomeReverted = "reverted"

	maxConfirmationDelay = time.Hour
	listingLimit         = 500
)

var (
	ErrInvalidTransfer = errors.New("invalid simulated transfer")
	senderAddress      = tron.SimulatedJarAddress(0)
)

type Provider struct {
	db database.Querier
}

func NewProvider(db database.Querier) *Provider {
	return &Provider{db: db}
}

func (p *Provider) Network() tron.Network {
	return tron.NetworkSimulated
}

func (p *Provider) ListIncoming(ctx context.Context, to tron.Address, since time.Time) (chain.Listing, error) {
	rows, err := p.db.Query(ctx, `
		SELECT tx_hash, from_address, to_address, amount, created_at
		FROM simulated_chain_transfers
		WHERE to_address = $1 AND created_at >= $2
		ORDER BY created_at, tx_hash
		LIMIT $3`,
		to, since, listingLimit+1)
	if err != nil {
		return chain.Listing{}, fmt.Errorf("list simulated transfers: %w", err)
	}
	observations, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (chain.Observation, error) {
		var (
			observation chain.Observation
			amount      int64
		)
		err := row.Scan(&observation.TxHash, &observation.From, &observation.To, &amount, &observation.BlockTime)
		observation.Amount = usdt.Amount(amount)
		observation.BlockTime = observation.BlockTime.UTC()
		return observation, err
	})
	if err != nil {
		return chain.Listing{}, fmt.Errorf("read simulated transfers: %w", err)
	}
	listing := chain.Listing{Observations: observations, Complete: len(observations) <= listingLimit}
	if !listing.Complete {
		listing.Observations = observations[:listingLimit]
	}
	for _, observation := range listing.Observations {
		if observation.BlockTime.After(listing.NewestTimestamp) {
			listing.NewestTimestamp = observation.BlockTime
		}
	}
	return listing, nil
}

func (p *Provider) TransactionOutcome(ctx context.Context, txHash string) (chain.Outcome, error) {
	hash, err := chain.NormalizeTxHash(txHash)
	if err != nil {
		return chain.Outcome{}, err
	}
	var (
		transfer chain.TokenTransfer
		amount   int64
		outcome  string
		final    bool
		result   chain.Outcome
	)
	err = p.db.QueryRow(ctx, `
		SELECT block_number, created_at, log_index, from_address, to_address, amount, outcome, solidifies_at <= now()
		FROM simulated_chain_transfers WHERE tx_hash = $1`,
		hash,
	).Scan(&result.BlockNumber, &result.BlockTime, &transfer.LogIndex, &transfer.From, &transfer.To, &amount, &outcome, &final)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !final) {
		return chain.Outcome{State: chain.OutcomeNotFinal}, nil
	}
	if err != nil {
		return chain.Outcome{}, fmt.Errorf("load simulated transfer: %w", err)
	}
	result.BlockTime = result.BlockTime.UTC()
	if outcome == OutcomeReverted {
		result.State = chain.OutcomeFailed
		result.FailureReason = "receipt_revert"
		return result, nil
	}
	transfer.Amount = usdt.Amount(amount)
	result.State = chain.OutcomeFinal
	result.Transfers = []chain.TokenTransfer{transfer}
	return result, nil
}

func (p *Provider) TokenBalance(ctx context.Context, address tron.Address) (usdt.Amount, error) {
	var balance int64
	err := p.db.QueryRow(ctx, `
		SELECT coalesce(sum(CASE WHEN to_address = $1 THEN amount ELSE -amount END), 0)::bigint
		FROM simulated_chain_transfers
		WHERE outcome = 'success' AND solidifies_at <= now() AND (to_address = $1 OR from_address = $1)`,
		address,
	).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("simulated balance: %w", err)
	}
	return usdt.Amount(balance), nil
}

type Transfer struct {
	To                tron.Address
	From              tron.Address
	Amount            usdt.Amount
	Outcome           string
	ConfirmationDelay time.Duration
}

type SentTransfer struct {
	TxHash       string
	From         tron.Address
	To           tron.Address
	Amount       usdt.Amount
	Outcome      string
	CreatedAt    time.Time
	SolidifiesAt time.Time
	BlockNumber  int64
}

func Send(ctx context.Context, db database.Querier, transfer Transfer) (SentTransfer, error) {
	if !transfer.Amount.IsPositive() {
		return SentTransfer{}, fmt.Errorf("%w: amount must be positive", ErrInvalidTransfer)
	}
	if _, err := tron.ParseAddress(string(transfer.To)); err != nil {
		return SentTransfer{}, fmt.Errorf("%w: recipient: %v", ErrInvalidTransfer, err)
	}
	if transfer.From == "" {
		transfer.From = senderAddress
	}
	if _, err := tron.ParseAddress(string(transfer.From)); err != nil {
		return SentTransfer{}, fmt.Errorf("%w: sender: %v", ErrInvalidTransfer, err)
	}
	if transfer.Outcome == "" {
		transfer.Outcome = OutcomeSuccess
	}
	if transfer.Outcome != OutcomeSuccess && transfer.Outcome != OutcomeReverted {
		return SentTransfer{}, fmt.Errorf("%w: outcome must be success or reverted", ErrInvalidTransfer)
	}
	if transfer.ConfirmationDelay < 0 || transfer.ConfirmationDelay > maxConfirmationDelay {
		return SentTransfer{}, fmt.Errorf("%w: confirmation delay must be between 0 and %s", ErrInvalidTransfer, maxConfirmationDelay)
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return SentTransfer{}, fmt.Errorf("generate simulated tx hash: %w", err)
	}
	sent := SentTransfer{TxHash: hex.EncodeToString(random), From: transfer.From, To: transfer.To, Amount: transfer.Amount, Outcome: transfer.Outcome}
	err := db.QueryRow(ctx, `
		INSERT INTO simulated_chain_transfers (tx_hash, from_address, to_address, amount, outcome, solidifies_at)
		VALUES ($1, $2, $3, $4, $5, now() + make_interval(secs => $6))
		RETURNING block_number, created_at, solidifies_at`,
		sent.TxHash, sent.From, sent.To, int64(sent.Amount), sent.Outcome, transfer.ConfirmationDelay.Seconds(),
	).Scan(&sent.BlockNumber, &sent.CreatedAt, &sent.SolidifiesAt)
	if err != nil {
		return SentTransfer{}, fmt.Errorf("record simulated transfer: %w", err)
	}
	return sent, nil
}
