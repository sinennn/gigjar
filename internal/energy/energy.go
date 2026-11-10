package energy

import (
	"context"
	"errors"

	"gigjar/internal/tron"
)

var (
	ErrTooExpensive = errors.New("energy rental quote is above the configured maximum")
	ErrUnavailable  = errors.New("energy rental market cannot fill the order")
)

type Quote struct {
	UnitPriceSun float64
	TotalSun     int64
	Available    int64
}

type Rental struct {
	Reference    string
	Amount       int64
	TotalSun     int64
	UnitPriceSun float64
	Provider     string
}

type PaymentSigner func(ctx context.Context, to tron.Address, amountSun int64) (signedTx []byte, txID string, err error)

type Renter interface {
	Name() string
	Rent(ctx context.Context, receiver tron.Address, amount int64) (Rental, error)
}
