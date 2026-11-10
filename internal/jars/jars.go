package jars

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/database"
	"gigjar/internal/ledger"
	"gigjar/internal/transfers"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var (
	ErrJarNotFound         = errors.New("jar not found")
	ErrDestinationNotFound = errors.New("no Cleva destination configured")
	ErrAlreadyProvisioned  = errors.New("jar already has an address")
)

type Jar struct {
	UserID          uuid.UUID
	Network         tron.Network
	DerivationIndex int64
	Address         *tron.Address
}

type Summary struct {
	Jar                      Jar
	Balances                 ledger.Balances
	Pending                  usdt.Amount
	SettlementThreshold      usdt.Amount
	SettlementFee            usdt.Amount
	RemainingUntilSettlement usdt.Amount
	ClevaDestinationSet      bool
}

type Destination struct {
	Address   tron.Address
	CreatedAt time.Time
}

type Service struct {
	pool                *pgxpool.Pool
	settlementThreshold usdt.Amount
	settlementFee       usdt.Amount
}

func NewService(pool *pgxpool.Pool, settlementThreshold, settlementFee usdt.Amount) *Service {
	return &Service{pool: pool, settlementThreshold: settlementThreshold, settlementFee: settlementFee}
}

func Create(ctx context.Context, tx pgx.Tx, userID uuid.UUID, network tron.Network) error {
	var derivationIndex int64
	err := tx.QueryRow(ctx,
		`INSERT INTO jars (user_id, network) VALUES ($1, $2) RETURNING derivation_index`,
		userID, network,
	).Scan(&derivationIndex)
	if err != nil {
		return fmt.Errorf("create jar: %w", err)
	}
	if network.IsSimulated() {
		return Provision(ctx, tx, userID, tron.SimulatedJarAddress(derivationIndex))
	}
	return nil
}

func Provision(ctx context.Context, tx pgx.Tx, userID uuid.UUID, address tron.Address) error {
	if _, err := tron.ParseAddress(string(address)); err != nil {
		return err
	}
	var network tron.Network
	err := tx.QueryRow(ctx, `
		UPDATE jars SET address = $2, provisioned_at = now()
		WHERE user_id = $1 AND address IS NULL
		RETURNING network`,
		userID, address,
	).Scan(&network)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := Get(ctx, tx, userID); getErr != nil {
			return getErr
		}
		return ErrAlreadyProvisioned
	}
	if err != nil {
		return fmt.Errorf("provision jar address: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO jar_watch_state (user_id, network, address, min_block_timestamp_ms)
		VALUES ($1, $2, $3, (extract(epoch FROM now()) * 1000)::bigint)`,
		userID, network, address)
	if err != nil {
		return fmt.Errorf("start watching jar address: %w", err)
	}
	return nil
}

const wakeSpacing = 10 * time.Second

func Wake(ctx context.Context, db database.Querier, userID uuid.UUID) error {
	_, err := db.Exec(ctx, `
		UPDATE jar_watch_state
		SET next_poll_at = greatest(now(), coalesce(last_polled_at, now()) + make_interval(secs => $2))
		WHERE user_id = $1
		  AND next_poll_at > greatest(now(), coalesce(last_polled_at, now()) + make_interval(secs => $2))`,
		userID, wakeSpacing.Seconds())
	if err != nil {
		return fmt.Errorf("wake jar watcher: %w", err)
	}
	return nil
}

func ProvisionMissingSimulated(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx,
		`SELECT user_id, derivation_index FROM jars WHERE network = 'simulated' AND address IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("find unprovisioned simulated jars: %w", err)
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
		err := database.WithTx(ctx, pool, func(tx pgx.Tx) error {
			return Provision(ctx, tx, item.userID, tron.SimulatedJarAddress(item.index))
		})
		if errors.Is(err, ErrAlreadyProvisioned) {
			continue
		}
		if err != nil {
			return provisioned, err
		}
		provisioned++
	}
	return provisioned, nil
}

func Get(ctx context.Context, db database.Querier, userID uuid.UUID) (Jar, error) {
	var jar Jar
	err := db.QueryRow(ctx,
		`SELECT user_id, network, derivation_index, address FROM jars WHERE user_id = $1`,
		userID,
	).Scan(&jar.UserID, &jar.Network, &jar.DerivationIndex, &jar.Address)
	if errors.Is(err, pgx.ErrNoRows) {
		return Jar{}, ErrJarNotFound
	}
	if err != nil {
		return Jar{}, fmt.Errorf("load jar: %w", err)
	}
	return jar, nil
}

func (s *Service) Summary(ctx context.Context, userID uuid.UUID) (Summary, error) {
	jar, err := Get(ctx, s.pool, userID)
	if err != nil {
		return Summary{}, err
	}
	balances, err := ledger.UserBalances(ctx, s.pool, userID)
	if err != nil {
		return Summary{}, err
	}
	pending, err := transfers.PendingTotal(ctx, s.pool, userID)
	if err != nil {
		return Summary{}, err
	}
	_, destinationErr := s.CurrentDestination(ctx, userID)
	if destinationErr != nil && !errors.Is(destinationErr, ErrDestinationNotFound) {
		return Summary{}, destinationErr
	}
	return Summary{
		Jar:                      jar,
		Balances:                 balances,
		Pending:                  pending,
		SettlementThreshold:      s.settlementThreshold,
		SettlementFee:            s.settlementFee,
		RemainingUntilSettlement: max(s.settlementThreshold-balances.Available, 0),
		ClevaDestinationSet:      destinationErr == nil,
	}, nil
}

func (s *Service) SetDestination(ctx context.Context, userID uuid.UUID, address tron.Address) (Destination, error) {
	destination := Destination{Address: address}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO cleva_destinations (user_id, address) VALUES ($1, $2) RETURNING created_at`,
		userID, address,
	).Scan(&destination.CreatedAt)
	if err != nil {
		return Destination{}, fmt.Errorf("save Cleva destination: %w", err)
	}
	return destination, nil
}

func (s *Service) CurrentDestination(ctx context.Context, userID uuid.UUID) (Destination, error) {
	return CurrentDestination(ctx, s.pool, userID)
}

func CurrentDestination(ctx context.Context, db database.Querier, userID uuid.UUID) (Destination, error) {
	var destination Destination
	err := db.QueryRow(ctx, `
		SELECT address, created_at FROM cleva_destinations
		WHERE user_id = $1 ORDER BY id DESC LIMIT 1`,
		userID,
	).Scan(&destination.Address, &destination.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Destination{}, ErrDestinationNotFound
	}
	if err != nil {
		return Destination{}, fmt.Errorf("load Cleva destination: %w", err)
	}
	return destination, nil
}
