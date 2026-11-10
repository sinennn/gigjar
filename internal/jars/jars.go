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
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var (
	ErrJarNotFound         = errors.New("jar not found")
	ErrDestinationNotFound = errors.New("no Cleva destination configured")
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
	SettlementThreshold      usdt.Amount
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
}

func NewService(pool *pgxpool.Pool, settlementThreshold usdt.Amount) *Service {
	return &Service{pool: pool, settlementThreshold: settlementThreshold}
}

func Create(ctx context.Context, tx pgx.Tx, userID uuid.UUID, network tron.Network) error {
	_, err := tx.Exec(ctx, `INSERT INTO jars (user_id, network) VALUES ($1, $2)`, userID, network)
	if err != nil {
		return fmt.Errorf("create jar: %w", err)
	}
	return nil
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
	_, destinationErr := s.CurrentDestination(ctx, userID)
	if destinationErr != nil && !errors.Is(destinationErr, ErrDestinationNotFound) {
		return Summary{}, destinationErr
	}
	return Summary{
		Jar:                      jar,
		Balances:                 balances,
		SettlementThreshold:      s.settlementThreshold,
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
