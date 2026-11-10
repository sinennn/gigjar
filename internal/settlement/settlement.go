package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain"
	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet"
)

type State string

const (
	StateQueued    State = "queued"
	StateSigning   State = "signing"
	StateBroadcast State = "broadcast"
	StateConfirmed State = "confirmed"
	StateFailed    State = "failed"
)

const (
	ReasonReverted          = "transaction_failed_on_chain"
	ReasonDropped           = "transaction_expired_unconfirmed"
	ReasonPreparationFailed = "preparation_failed"
	ReasonSigningFailed     = "signing_failed"
)

var ClevaMinimumDeposit = usdt.MustParse("10")

type Config struct {
	Threshold       usdt.Amount
	Fee             usdt.Amount
	Tick            time.Duration
	BatchSize       int
	Lease           time.Duration
	MaxAttempts     int
	RetryBase       time.Duration
	RetryMax        time.Duration
	PollInterval    time.Duration
	BroadcastGrace  time.Duration
	FailureCooldown time.Duration
	MaxDailyFailure int
}

func DefaultConfig() Config {
	return Config{
		Threshold:       usdt.MustParse("12"),
		Fee:             usdt.MustParse("1"),
		Tick:            3 * time.Second,
		BatchSize:       20,
		Lease:           2 * time.Minute,
		MaxAttempts:     8,
		RetryBase:       5 * time.Second,
		RetryMax:        5 * time.Minute,
		PollInterval:    5 * time.Second,
		BroadcastGrace:  2 * time.Minute,
		FailureCooldown: 10 * time.Minute,
		MaxDailyFailure: 5,
	}
}

func (c Config) Validate() error {
	if c.Fee < 0 {
		return errors.New("settlement fee must not be negative")
	}
	if c.Threshold-c.Fee <= ClevaMinimumDeposit {
		return fmt.Errorf("threshold %s minus fee %s must stay above Cleva's %s minimum deposit", c.Threshold, c.Fee, ClevaMinimumDeposit)
	}
	return nil
}

type Sweeper struct {
	pool    *pgxpool.Pool
	wallet  wallet.Provider
	chain   chain.Provider
	network tron.Network
	logger  *slog.Logger
	cfg     Config
}

func New(pool *pgxpool.Pool, w wallet.Provider, c chain.Provider, logger *slog.Logger, cfg Config) (*Sweeper, error) {
	if w.Network() != c.Network() {
		return nil, fmt.Errorf("wallet network %s does not match chain network %s", w.Network(), c.Network())
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Sweeper{pool: pool, wallet: w, chain: c, network: w.Network(), logger: logger, cfg: cfg}, nil
}

func (s *Sweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Tick)
	defer ticker.Stop()
	for {
		if _, err := s.ProvisionOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("provisioning pass failed", "error", err)
		}
		if _, err := s.QueueOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("queueing pass failed", "error", err)
		}
		if _, err := s.AdvanceOnce(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("advance pass failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Sweeper) ProvisionOnce(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT user_id, derivation_index FROM jars WHERE network = $1 AND address IS NULL ORDER BY derivation_index LIMIT $2`,
		s.network, s.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("find unprovisioned jars: %w", err)
	}
	type pending struct {
		userID uuid.UUID
		index  int64
	}
	missing, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pending, error) {
		var item pending
		err := row.Scan(&item.userID, &item.index)
		return item, err
	})
	if err != nil {
		return 0, err
	}
	provisioned := 0
	for _, item := range missing {
		address, err := s.wallet.JarAddress(ctx, item.index)
		if err != nil {
			return provisioned, fmt.Errorf("derive jar address %d: %w", item.index, err)
		}
		err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
			return jars.Provision(ctx, tx, item.userID, address)
		})
		if errors.Is(err, jars.ErrAlreadyProvisioned) {
			continue
		}
		if err != nil {
			return provisioned, err
		}
		provisioned++
		s.logger.Info("jar address provisioned", "user_id", item.userID, "derivation_index", item.index, "address", address)
	}
	return provisioned, nil
}

type candidate struct {
	userID        uuid.UUID
	jarAddress    tron.Address
	jarIndex      int64
	destinationID int64
	destination   tron.Address
}

func (s *Sweeper) QueueOnce(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.user_id, j.address, j.derivation_index, d.id, d.address
		FROM ledger_accounts a
		JOIN jars j ON j.user_id = a.user_id AND j.network = $1 AND j.address IS NOT NULL
		JOIN LATERAL (
			SELECT id, address FROM cleva_destinations WHERE user_id = a.user_id ORDER BY id DESC LIMIT 1
		) d ON true
		WHERE a.kind = 'jar_available' AND a.balance >= $2
		  AND NOT EXISTS (
			SELECT 1 FROM settlements s WHERE s.user_id = a.user_id AND s.state IN ('queued', 'signing', 'broadcast')
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM settlements s
			WHERE s.user_id = a.user_id AND s.state = 'failed'
			  AND s.failed_at > now() - make_interval(secs => $3)
		  )
		  AND (
			SELECT count(*) FROM settlements s
			WHERE s.user_id = a.user_id AND s.state = 'failed' AND s.failed_at > now() - interval '24 hours'
		  ) < $4
		ORDER BY a.balance DESC
		LIMIT $5`,
		s.network, int64(s.cfg.Threshold), s.cfg.FailureCooldown.Seconds(), s.cfg.MaxDailyFailure, s.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("find jars ready to sweep: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (candidate, error) {
		var c candidate
		err := row.Scan(&c.userID, &c.jarAddress, &c.jarIndex, &c.destinationID, &c.destination)
		return c, err
	})
	if err != nil {
		return 0, fmt.Errorf("read sweep candidates: %w", err)
	}

	queued := 0
	for _, c := range candidates {
		id, amount, err := s.queue(ctx, c)
		switch {
		case errors.Is(err, errNotEligible):
			continue
		case err != nil:
			s.logger.Error("queueing sweep failed", "user_id", c.userID, "error", err)
			continue
		}
		queued++
		s.logger.Info("sweep queued", "settlement_id", id, "user_id", c.userID, "amount", amount.String(),
			"fee", s.cfg.Fee.String(), "net", (amount - s.cfg.Fee).String(), "destination", c.destination)
	}
	return queued, nil
}

var errNotEligible = errors.New("jar is no longer eligible for a sweep")

func (s *Sweeper) queue(ctx context.Context, c candidate) (uuid.UUID, usdt.Amount, error) {
	var (
		id     uuid.UUID
		amount usdt.Amount
	)
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var balance int64
		err := tx.QueryRow(ctx,
			`SELECT balance FROM ledger_accounts WHERE user_id = $1 AND kind = 'jar_available' FOR UPDATE`, c.userID,
		).Scan(&balance)
		if err != nil {
			return fmt.Errorf("lock available balance: %w", err)
		}
		amount = usdt.Amount(balance)
		if amount < s.cfg.Threshold {
			return errNotEligible
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO settlements (user_id, network, jar_address, jar_index, destination_id, destination_address, amount, fee)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			c.userID, s.network, c.jarAddress, c.jarIndex, c.destinationID, c.destination, int64(amount), int64(s.cfg.Fee),
		).Scan(&id)
		if database.IsUniqueViolation(err, "settlements_one_in_flight_per_user") {
			return errNotEligible
		}
		if err != nil {
			return fmt.Errorf("create settlement: %w", err)
		}
		_, err = ledger.ReserveForSettlement(ctx, tx, c.userID, id, amount)
		return err
	})
	return id, amount, err
}

type Settlement struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Network         tron.Network
	JarAddress      tron.Address
	JarIndex        int64
	Destination     tron.Address
	Amount          usdt.Amount
	Fee             usdt.Amount
	State           State
	EnergyRequired  *int64
	EnergyOrderRef  *string
	EnergyOrderedAt *time.Time
	BurnFallback    bool
	FeeLimitSun     *int64
	TopUpTxID       *string
	TopUpSentAt     *time.Time
	TxID            *string
	SignedTx        json.RawMessage
	TxExpiresAt     *time.Time
	Attempts        int
	FailureReason   *string
	BlockNumber     *int64
	CreatedAt       time.Time
	ConfirmedAt     *time.Time
}

func (s Settlement) Net() usdt.Amount {
	return s.Amount - s.Fee
}

const settlementColumns = `id, user_id, network, jar_address, jar_index, destination_address, amount, fee, state,
	energy_required, energy_order_ref, energy_ordered_at, burn_fallback, fee_limit_sun, topup_tx_id, topup_sent_at,
	tx_id, signed_tx, tx_expires_at,
	attempts, failure_reason, block_number, created_at, confirmed_at`

func scanSettlement(row pgx.Row) (Settlement, error) {
	var (
		s           Settlement
		amount, fee int64
	)
	err := row.Scan(&s.ID, &s.UserID, &s.Network, &s.JarAddress, &s.JarIndex, &s.Destination, &amount, &fee, &s.State,
		&s.EnergyRequired, &s.EnergyOrderRef, &s.EnergyOrderedAt, &s.BurnFallback, &s.FeeLimitSun, &s.TopUpTxID, &s.TopUpSentAt,
		&s.TxID, &s.SignedTx,
		&s.TxExpiresAt, &s.Attempts, &s.FailureReason, &s.BlockNumber, &s.CreatedAt, &s.ConfirmedAt)
	s.Amount, s.Fee = usdt.Amount(amount), usdt.Amount(fee)
	return s, err
}

func (s *Sweeper) AdvanceOnce(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE settlements t SET next_attempt_at = now() + make_interval(secs => $3)
		FROM (
			SELECT id FROM settlements
			WHERE network = $1 AND state IN ('queued', 'signing', 'broadcast') AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		) due
		WHERE t.id = due.id
		RETURNING `+prefixed("t", settlementColumns),
		s.network, s.cfg.BatchSize, s.cfg.Lease.Seconds())
	if err != nil {
		return 0, fmt.Errorf("claim settlements: %w", err)
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Settlement, error) { return scanSettlement(row) })
	if err != nil {
		return 0, fmt.Errorf("read claimed settlements: %w", err)
	}
	advanced := 0
	for _, item := range due {
		if ctx.Err() != nil {
			return advanced, ctx.Err()
		}
		if err := s.step(ctx, item); err != nil {
			s.logger.Warn("settlement step failed", "settlement_id", item.ID, "state", item.State, "error", err)
			continue
		}
		advanced++
	}
	return advanced, nil
}

func (s *Sweeper) step(ctx context.Context, item Settlement) error {
	switch item.State {
	case StateQueued:
		return s.prepare(ctx, item)
	case StateSigning:
		return s.signAndBroadcast(ctx, item)
	case StateBroadcast:
		return s.track(ctx, item)
	default:
		return fmt.Errorf("settlement %s is in terminal state %s", item.ID, item.State)
	}
}

func (s *Sweeper) prepare(ctx context.Context, item Settlement) error {
	preparation := &wallet.Preparation{
		SettlementID:    item.ID,
		JarIndex:        item.JarIndex,
		Jar:             item.JarAddress,
		Destination:     item.Destination,
		NetAmount:       item.Net(),
		EnergyRequired:  item.EnergyRequired,
		EnergyOrderRef:  item.EnergyOrderRef,
		EnergyOrderedAt: item.EnergyOrderedAt,
		BurnFallback:    item.BurnFallback,
		FeeLimitSun:     item.FeeLimitSun,
		TopUpTxID:       item.TopUpTxID,
		TopUpSentAt:     item.TopUpSentAt,
	}
	readiness, prepErr := s.wallet.PrepareSweep(ctx, preparation)
	if err := s.savePreparation(ctx, item.ID, preparation); err != nil {
		return errors.Join(prepErr, err)
	}
	if prepErr != nil {
		return s.retryOrFail(ctx, item, ReasonPreparationFailed, prepErr)
	}
	if !readiness.Ready {
		delay := readiness.RetryAfter
		if delay <= 0 {
			delay = s.cfg.PollInterval
		}
		s.logger.Info("sweep waiting on resources", "settlement_id", item.ID, "note", readiness.Note, "retry_in", delay.String())
		return s.reschedule(ctx, item.ID, delay, "")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE settlements SET state = 'signing', resources_ready_at = now(), next_attempt_at = now(), last_error = NULL
		WHERE id = $1 AND state = 'queued'`, item.ID)
	if err != nil {
		return fmt.Errorf("mark settlement ready to sign: %w", err)
	}
	s.logger.Info("sweep resources ready", "settlement_id", item.ID, "note", readiness.Note)
	item.State = StateSigning
	item.FeeLimitSun = preparation.FeeLimitSun
	return s.signAndBroadcast(ctx, item)
}

func (s *Sweeper) savePreparation(ctx context.Context, id uuid.UUID, p *wallet.Preparation) error {
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE settlements
			SET energy_required = $2, energy_order_ref = $3, energy_ordered_at = $4, burn_fallback = $5, fee_limit_sun = $6,
			    topup_tx_id = $7, topup_sent_at = $8
			WHERE id = $1 AND state = 'queued'`,
			id, p.EnergyRequired, p.EnergyOrderRef, p.EnergyOrderedAt, p.BurnFallback, p.FeeLimitSun, p.TopUpTxID, p.TopUpSentAt)
		if err != nil {
			return fmt.Errorf("save preparation: %w", err)
		}
		for _, expense := range p.Expenses {
			detail, err := json.Marshal(expense.Detail)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO settlement_expenses (settlement_id, kind, trx_sun, reference, detail)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (kind, reference) DO NOTHING`,
				id, expense.Kind, expense.TRXSun, expense.Reference, detail)
			if err != nil {
				return fmt.Errorf("record expense: %w", err)
			}
		}
		p.Expenses = nil
		return nil
	})
}

func (s *Sweeper) signAndBroadcast(ctx context.Context, item Settlement) error {
	if item.TxID == nil {
		feeLimit := int64(0)
		if item.FeeLimitSun != nil {
			feeLimit = *item.FeeLimitSun
		}
		signed, err := s.wallet.SignSweep(ctx, wallet.SweepRequest{
			SettlementID: item.ID,
			JarIndex:     item.JarIndex,
			Jar:          item.JarAddress,
			Destination:  item.Destination,
			NetAmount:    item.Net(),
			FeeLimitSun:  feeLimit,
		})
		if err != nil {
			return s.retryOrFail(ctx, item, ReasonSigningFailed, err)
		}
		if _, err := chain.NormalizeTxHash(signed.TxID); err != nil {
			return s.retryOrFail(ctx, item, ReasonSigningFailed, fmt.Errorf("signer returned invalid tx id: %w", err))
		}
		tag, err := s.pool.Exec(ctx, `
			UPDATE settlements SET tx_id = $2, signed_tx = $3, tx_expires_at = $4
			WHERE id = $1 AND state = 'signing' AND tx_id IS NULL`,
			item.ID, signed.TxID, signed.Raw, signed.ExpiresAt)
		if err != nil {
			return fmt.Errorf("persist signed transaction: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("settlement %s changed while signing", item.ID)
		}
		item.TxID, item.SignedTx, item.TxExpiresAt = &signed.TxID, signed.Raw, &signed.ExpiresAt
		s.logger.Info("sweep signed", "settlement_id", item.ID, "tx_id", signed.TxID, "expires_at", signed.ExpiresAt)
	}

	signed := wallet.SignedTx{TxID: *item.TxID, Raw: item.SignedTx, ExpiresAt: *item.TxExpiresAt}
	if err := s.wallet.Broadcast(ctx, signed); err != nil {
		if time.Now().After(signed.ExpiresAt.Add(s.cfg.BroadcastGrace)) {
			return s.resolveExpired(ctx, item, err)
		}
		s.logger.Warn("broadcast failed; will retry while the signature is valid", "settlement_id", item.ID, "tx_id", signed.TxID, "error", err)
		return s.reschedule(ctx, item.ID, s.cfg.PollInterval, err.Error())
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE settlements SET state = 'broadcast', broadcast_at = coalesce(broadcast_at, now()),
		       next_attempt_at = now() + make_interval(secs => $2), last_error = NULL
		WHERE id = $1 AND state IN ('signing', 'broadcast')`,
		item.ID, s.cfg.PollInterval.Seconds())
	if err != nil {
		return fmt.Errorf("mark settlement broadcast: %w", err)
	}
	s.logger.Info("sweep broadcast", "settlement_id", item.ID, "tx_id", signed.TxID)
	return nil
}

func (s *Sweeper) track(ctx context.Context, item Settlement) error {
	outcome, err := s.chain.TransactionOutcome(ctx, *item.TxID)
	if err != nil {
		return s.reschedule(ctx, item.ID, s.cfg.PollInterval, err.Error())
	}
	switch outcome.State {
	case chain.OutcomeFinal:
		for _, transfer := range outcome.Transfers {
			if transfer.From == item.JarAddress && transfer.To == item.Destination && transfer.Amount == item.Net() {
				return s.confirm(ctx, item, outcome)
			}
		}
		s.logger.Error("sweep transaction is final but does not contain the expected transfer; needs manual review",
			"settlement_id", item.ID, "tx_id", *item.TxID)
		return s.reschedule(ctx, item.ID, time.Hour, "final transaction without the expected USDT transfer: manual review required")
	case chain.OutcomeFailed:
		return s.fail(ctx, item, ReasonReverted+":"+outcome.FailureReason)
	default:
		if time.Now().Before(item.TxExpiresAt.Add(s.cfg.BroadcastGrace)) {
			known, err := s.wallet.TransactionKnown(ctx, *item.TxID)
			if err == nil && !known {
				if err := s.wallet.Broadcast(ctx, wallet.SignedTx{TxID: *item.TxID, Raw: item.SignedTx, ExpiresAt: *item.TxExpiresAt}); err != nil {
					s.logger.Warn("rebroadcast failed", "settlement_id", item.ID, "error", err)
				}
			}
			return s.reschedule(ctx, item.ID, s.cfg.PollInterval, "")
		}
		return s.resolveExpired(ctx, item, nil)
	}
}

func (s *Sweeper) resolveExpired(ctx context.Context, item Settlement, cause error) error {
	known, err := s.wallet.TransactionKnown(ctx, *item.TxID)
	if err != nil {
		return s.reschedule(ctx, item.ID, s.cfg.PollInterval, err.Error())
	}
	if known {
		if item.State != StateBroadcast {
			if _, err := s.pool.Exec(ctx, `UPDATE settlements SET state = 'broadcast', broadcast_at = coalesce(broadcast_at, now()) WHERE id = $1 AND state = 'signing'`, item.ID); err != nil {
				return err
			}
		}
		return s.reschedule(ctx, item.ID, s.cfg.PollInterval, "")
	}
	if cause != nil {
		s.logger.Warn("signed sweep expired without reaching the chain", "settlement_id", item.ID, "error", cause)
	}
	return s.fail(ctx, item, ReasonDropped)
}

func (s *Sweeper) confirm(ctx context.Context, item Settlement, outcome chain.Outcome) error {
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE settlements SET state = 'confirmed', confirmed_at = now(), block_number = $2, last_error = NULL
			WHERE id = $1 AND state = 'broadcast'`,
			item.ID, outcome.BlockNumber)
		if err != nil {
			return fmt.Errorf("mark settlement confirmed: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return nil
		}
		_, err = ledger.CompleteSettlement(ctx, tx, item.UserID, item.ID, item.Amount, item.Fee)
		return err
	})
	if err != nil {
		return err
	}
	s.logger.Info("sweep confirmed", "settlement_id", item.ID, "tx_id", *item.TxID, "user_id", item.UserID,
		"net", item.Net().String(), "fee", item.Fee.String(), "block", outcome.BlockNumber)
	return nil
}

func (s *Sweeper) fail(ctx context.Context, item Settlement, reason string) error {
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE settlements SET state = 'failed', failed_at = now(), failure_reason = $2
			WHERE id = $1 AND state IN ('queued', 'signing', 'broadcast')`,
			item.ID, reason)
		if err != nil {
			return fmt.Errorf("mark settlement failed: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return nil
		}
		_, err = ledger.ReleaseSettlement(ctx, tx, item.UserID, item.ID, item.Amount)
		return err
	})
	if err != nil {
		return err
	}
	s.logger.Warn("sweep failed; funds returned to the jar", "settlement_id", item.ID, "user_id", item.UserID, "reason", reason)
	return nil
}

func (s *Sweeper) retryOrFail(ctx context.Context, item Settlement, reason string, cause error) error {
	attempts := item.Attempts + 1
	if errors.Is(cause, wallet.ErrPermanent) || attempts >= s.cfg.MaxAttempts {
		if item.TxID != nil {
			return s.reschedule(ctx, item.ID, s.cfg.PollInterval, cause.Error())
		}
		s.logger.Warn("giving up on sweep", "settlement_id", item.ID, "attempts", attempts, "error", cause)
		return s.fail(ctx, item, reason)
	}
	delay := s.cfg.RetryBase
	for step := 1; step < attempts && delay < s.cfg.RetryMax; step++ {
		delay *= 2
	}
	delay = min(delay, s.cfg.RetryMax)
	_, err := s.pool.Exec(ctx, `
		UPDATE settlements SET attempts = $2, last_error = left($3, 500), next_attempt_at = now() + make_interval(secs => $4)
		WHERE id = $1 AND state IN ('queued', 'signing')`,
		item.ID, attempts, cause.Error(), delay.Seconds())
	if err != nil {
		return err
	}
	return cause
}

func (s *Sweeper) reschedule(ctx context.Context, id uuid.UUID, delay time.Duration, lastError string) error {
	var lastErr *string
	if lastError != "" {
		lastErr = &lastError
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE settlements SET next_attempt_at = now() + make_interval(secs => $2), last_error = left($3, 500)
		WHERE id = $1 AND state IN ('queued', 'signing', 'broadcast')`,
		id, delay.Seconds(), lastErr)
	return err
}

func prefixed(alias, columns string) string {
	parts := strings.Split(columns, ",")
	for index, part := range parts {
		parts[index] = alias + "." + strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}
