package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var ErrPermanent = errors.New("permanent wallet error")

type SignedTx struct {
	TxID      string
	Raw       json.RawMessage
	ExpiresAt time.Time
}

type Expense struct {
	Kind      string
	TRXSun    int64
	Reference string
	Detail    map[string]any
}

type Preparation struct {
	SettlementID    uuid.UUID
	JarIndex        int64
	Jar             tron.Address
	Destination     tron.Address
	NetAmount       usdt.Amount
	EnergyRequired  *int64
	EnergyOrderRef  *string
	EnergyOrderedAt *time.Time
	BurnFallback    bool
	FeeLimitSun     *int64
	TopUpTxID       *string
	TopUpSentAt     *time.Time
	Expenses        []Expense
}

type Readiness struct {
	Ready      bool
	RetryAfter time.Duration
	Note       string
}

type SweepRequest struct {
	SettlementID uuid.UUID
	JarIndex     int64
	Jar          tron.Address
	Destination  tron.Address
	NetAmount    usdt.Amount
	FeeLimitSun  int64
}

type Provider interface {
	Network() tron.Network
	JarAddress(ctx context.Context, index int64) (tron.Address, error)
	PrepareSweep(ctx context.Context, preparation *Preparation) (Readiness, error)
	SignSweep(ctx context.Context, request SweepRequest) (SignedTx, error)
	Broadcast(ctx context.Context, tx SignedTx) error
	TransactionKnown(ctx context.Context, txID string) (bool, error)
}
