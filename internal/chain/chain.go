package chain

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var (
	ErrRateLimited  = errors.New("chain provider rate limited the request")
	ErrInvalidTxID  = errors.New("transaction id must be 64 hex characters")
	txHashPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	ErrUnavailable  = errors.New("chain provider unavailable")
	ErrMalformed    = errors.New("chain provider returned a malformed response")
	ErrNotSupported = errors.New("operation not supported by this chain provider")
)

type Observation struct {
	TxHash    string
	From      tron.Address
	To        tron.Address
	Amount    usdt.Amount
	BlockTime time.Time
}

type Listing struct {
	Observations    []Observation
	NewestTimestamp time.Time
	Complete        bool
}

type OutcomeState string

const (
	OutcomeNotFinal OutcomeState = "not_final"
	OutcomeFailed   OutcomeState = "failed"
	OutcomeFinal    OutcomeState = "final"
)

type TokenTransfer struct {
	LogIndex int
	From     tron.Address
	To       tron.Address
	Amount   usdt.Amount
}

type Outcome struct {
	State         OutcomeState
	FailureReason string
	BlockNumber   int64
	BlockTime     time.Time
	Transfers     []TokenTransfer
}

type Provider interface {
	Network() tron.Network
	ListIncoming(ctx context.Context, to tron.Address, since time.Time) (Listing, error)
	TransactionOutcome(ctx context.Context, txHash string) (Outcome, error)
	TokenBalance(ctx context.Context, address tron.Address) (usdt.Amount, error)
}

func NormalizeTxHash(value string) (string, error) {
	hash := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "0x")
	if !txHashPattern.MatchString(hash) {
		return "", ErrInvalidTxID
	}
	return hash, nil
}

func (o Outcome) TransfersTo(address tron.Address) []TokenTransfer {
	var matched []TokenTransfer
	for _, transfer := range o.Transfers {
		if transfer.To == address {
			matched = append(matched, transfer)
		}
	}
	return matched
}
