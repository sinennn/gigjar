package watcher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain"
	"gigjar/internal/transfers"
	"gigjar/internal/tron"
)

type Config struct {
	Tick               time.Duration
	BatchSize          int
	ActivePollInterval time.Duration
	IdlePollInterval   time.Duration
	PollLease          time.Duration
	CursorOverlap      time.Duration
	ConfirmLease       time.Duration
	MinConfirmBackoff  time.Duration
	MaxConfirmBackoff  time.Duration
	DropAfter          time.Duration
	ReconcileInterval  time.Duration
	ReconcileBatch     int
}

func DefaultConfig() Config {
	return Config{
		Tick:               3 * time.Second,
		BatchSize:          25,
		ActivePollInterval: 15 * time.Second,
		IdlePollInterval:   5 * time.Minute,
		PollLease:          2 * time.Minute,
		CursorOverlap:      10 * time.Minute,
		ConfirmLease:       time.Minute,
		MinConfirmBackoff:  5 * time.Second,
		MaxConfirmBackoff:  2 * time.Minute,
		DropAfter:          24 * time.Hour,
		ReconcileInterval:  10 * time.Minute,
		ReconcileBatch:     50,
	}
}

type Watcher struct {
	pool     *pgxpool.Pool
	provider chain.Provider
	network  tron.Network
	logger   *slog.Logger
	cfg      Config
}

func New(pool *pgxpool.Pool, provider chain.Provider, logger *slog.Logger, cfg Config) *Watcher {
	return &Watcher{pool: pool, provider: provider, network: provider.Network(), logger: logger, cfg: cfg}
}

func (w *Watcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.Tick)
	defer ticker.Stop()
	var lastReconcile time.Time
	for {
		if _, err := w.DiscoverOnce(ctx); err != nil && ctx.Err() == nil {
			w.logger.Error("discovery pass failed", "error", err)
		}
		if _, err := w.ConfirmOnce(ctx); err != nil && ctx.Err() == nil {
			w.logger.Error("confirmation pass failed", "error", err)
		}
		if time.Since(lastReconcile) >= w.cfg.ReconcileInterval {
			report, err := w.ReconcileOnce(ctx)
			switch {
			case err != nil && ctx.Err() == nil:
				w.logger.Error("reconciliation failed", "error", err)
			case err == nil:
				report.Log(w.logger)
				lastReconcile = time.Now()
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

type dueJar struct {
	userID   uuid.UUID
	address  tron.Address
	cursorMs int64
	failures int
}

func (w *Watcher) DiscoverOnce(ctx context.Context) (int, error) {
	rows, err := w.pool.Query(ctx, `
		UPDATE jar_watch_state s
		SET next_poll_at = now() + make_interval(secs => $3)
		FROM (
			SELECT user_id FROM jar_watch_state
			WHERE network = $1 AND next_poll_at <= now()
			ORDER BY next_poll_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		) due
		WHERE s.user_id = due.user_id
		RETURNING s.user_id, s.address, s.min_block_timestamp_ms, s.consecutive_failures`,
		w.network, w.cfg.BatchSize, w.cfg.PollLease.Seconds())
	if err != nil {
		return 0, fmt.Errorf("claim jars to poll: %w", err)
	}
	jars, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dueJar, error) {
		var jar dueJar
		err := row.Scan(&jar.userID, &jar.address, &jar.cursorMs, &jar.failures)
		return jar, err
	})
	if err != nil {
		return 0, fmt.Errorf("read jars to poll: %w", err)
	}

	discovered := 0
	for _, jar := range jars {
		if ctx.Err() != nil {
			return discovered, ctx.Err()
		}
		count, err := w.pollJar(ctx, jar)
		discovered += count
		if err != nil {
			w.logger.Warn("polling jar failed", "user_id", jar.userID, "address", jar.address, "error", err)
		}
	}
	return discovered, nil
}

func (w *Watcher) pollJar(ctx context.Context, jar dueJar) (int, error) {
	since := time.UnixMilli(jar.cursorMs).Add(-w.cfg.CursorOverlap)
	listing, err := w.provider.ListIncoming(ctx, jar.address, since)
	if err != nil {
		backoff := min(w.cfg.ActivePollInterval*time.Duration(1<<min(jar.failures, 6)), 10*time.Minute)
		_, updateErr := w.pool.Exec(ctx, `
			UPDATE jar_watch_state
			SET consecutive_failures = consecutive_failures + 1, last_error = left($2, 500),
			    next_poll_at = now() + make_interval(secs => $3)
			WHERE user_id = $1`,
			jar.userID, err.Error(), backoff.Seconds())
		return 0, errors.Join(err, updateErr)
	}

	inserted, err := transfers.RecordObservations(ctx, w.pool, w.network, jar.userID, listing.Observations)
	if err != nil {
		return inserted, err
	}
	for _, observation := range listing.Observations {
		w.logger.Debug("observed transfer", "tx_hash", observation.TxHash, "to", observation.To, "amount", observation.Amount.String())
	}
	if inserted > 0 {
		w.logger.Info("new incoming transfers observed", "user_id", jar.userID, "count", inserted)
	}

	newestMs := jar.cursorMs
	if !listing.NewestTimestamp.IsZero() {
		newestMs = max(newestMs, listing.NewestTimestamp.UnixMilli())
	}
	_, err = w.pool.Exec(ctx, `
		UPDATE jar_watch_state
		SET min_block_timestamp_ms = $2, last_polled_at = now(), consecutive_failures = 0, last_error = NULL,
		    next_poll_at = now() + CASE
		        WHEN NOT $3 THEN interval '0'
		        WHEN EXISTS (SELECT 1 FROM payment_links WHERE user_id = $1 AND status = 'open')
		          OR EXISTS (SELECT 1 FROM incoming_transfers WHERE user_id = $1 AND status = 'pending')
		          OR last_polled_at IS NULL
		        THEN make_interval(secs => $4)
		        ELSE make_interval(secs => $5)
		    END
		WHERE user_id = $1`,
		jar.userID, newestMs, listing.Complete, w.cfg.ActivePollInterval.Seconds(), w.cfg.IdlePollInterval.Seconds())
	if err != nil {
		return inserted, fmt.Errorf("advance watch cursor: %w", err)
	}
	return inserted, nil
}

func (w *Watcher) ConfirmOnce(ctx context.Context) (int, error) {
	due, err := transfers.ClaimDue(ctx, w.pool, w.network, w.cfg.BatchSize, w.cfg.ConfirmLease)
	if err != nil {
		return 0, err
	}
	confirmed := 0
	for _, item := range due {
		if ctx.Err() != nil {
			return confirmed, ctx.Err()
		}
		done, err := w.resolve(ctx, item)
		if err != nil {
			w.logger.Warn("resolving transfer failed", "transfer_id", item.ID, "tx_hash", item.TxHash, "error", err)
			if rescheduleErr := transfers.Reschedule(ctx, w.pool, item.ID, w.backoff(item.Attempts)); rescheduleErr != nil {
				w.logger.Error("rescheduling transfer failed", "transfer_id", item.ID, "error", rescheduleErr)
			}
			continue
		}
		if done {
			confirmed++
		}
	}
	return confirmed, nil
}

func (w *Watcher) resolve(ctx context.Context, item transfers.Due) (bool, error) {
	outcome, err := w.provider.TransactionOutcome(ctx, item.TxHash)
	if err != nil {
		return false, err
	}
	switch outcome.State {
	case chain.OutcomeNotFinal:
		if time.Since(item.ObservedAt) > w.cfg.DropAfter {
			w.logger.Warn("transfer never solidified; marking failed", "transfer_id", item.ID, "tx_hash", item.TxHash)
			return false, transfers.MarkFailed(ctx, w.pool, item.ID, transfers.ReasonNotSolidified)
		}
		return false, transfers.Reschedule(ctx, w.pool, item.ID, w.backoff(item.Attempts))
	case chain.OutcomeFailed:
		w.logger.Info("transfer failed on chain", "transfer_id", item.ID, "tx_hash", item.TxHash, "reason", outcome.FailureReason)
		return false, transfers.MarkFailed(ctx, w.pool, item.ID, outcome.FailureReason)
	case chain.OutcomeFinal:
		confirmation, err := transfers.Confirm(ctx, w.pool, item.ID, outcome)
		if err != nil {
			return false, err
		}
		if confirmation.Status == transfers.StatusConfirmed && !confirmation.AlreadyDone {
			w.logger.Info("transfer confirmed and credited",
				"transfer_id", item.ID, "tx_hash", item.TxHash, "user_id", item.UserID,
				"amount", confirmation.Amount.String(), "payment_link_id", confirmation.PaymentLinkID)
			return true, nil
		}
		if confirmation.Status == transfers.StatusFailed {
			w.logger.Warn("solidified transaction has no USDT transfer to the jar", "transfer_id", item.ID, "tx_hash", item.TxHash)
		}
		return false, nil
	default:
		return false, fmt.Errorf("unknown outcome state %q", outcome.State)
	}
}

func (w *Watcher) backoff(attempts int) time.Duration {
	delay := w.cfg.MinConfirmBackoff
	for step := 1; step < attempts && delay < w.cfg.MaxConfirmBackoff; step++ {
		delay *= 2
	}
	return min(delay, w.cfg.MaxConfirmBackoff)
}
