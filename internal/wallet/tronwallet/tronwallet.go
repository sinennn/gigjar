package tronwallet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"gigjar/internal/chain/trongrid"
	"gigjar/internal/energy"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet"
)

type Chain interface {
	AccountResources(ctx context.Context, address tron.Address) (trongrid.AccountResources, error)
	EstimateUSDTTransferEnergy(ctx context.Context, from, to tron.Address, amount usdt.Amount) (int64, error)
	EnergyFeeSun(ctx context.Context) (int64, error)
	Broadcast(ctx context.Context, signed json.RawMessage) error
	TransactionKnown(ctx context.Context, txID string) (bool, error)
}

type Signer interface {
	JarAddress(ctx context.Context, index int64) (tron.Address, error)
	SignUSDTTransfer(ctx context.Context, fromIndex int64, to tron.Address, amount usdt.Amount, feeLimitSun int64) (wallet.SignedTx, tron.Address, error)
	SignTRXTransfer(ctx context.Context, to tron.Address, amountSun int64, purpose string) (wallet.SignedTx, error)
}

type Config struct {
	Network             tron.Network
	ActivationSun       int64
	AccountCreationSun  int64
	BandwidthReserveSun int64
	MinBandwidth        int64
	EnergyMargin        float64
	RentalTimeout       time.Duration
	TopUpWait           time.Duration
	PollInterval        time.Duration
	MaxFeeLimitSun      int64
}

func DefaultConfig(network tron.Network) Config {
	return Config{
		Network:             network,
		ActivationSun:       100_000,
		AccountCreationSun:  1_100_000,
		BandwidthReserveSun: 400_000,
		MinBandwidth:        400,
		EnergyMargin:        1.05,
		RentalTimeout:       3 * time.Minute,
		TopUpWait:           90 * time.Second,
		PollInterval:        5 * time.Second,
		MaxFeeLimitSun:      30_000_000,
	}
}

type Wallet struct {
	chain  Chain
	signer Signer
	renter energy.Renter
	logger *slog.Logger
	cfg    Config
	now    func() time.Time
}

func New(chain Chain, signer Signer, renter energy.Renter, logger *slog.Logger, cfg Config) *Wallet {
	return &Wallet{chain: chain, signer: signer, renter: renter, logger: logger, cfg: cfg, now: time.Now}
}

func (w *Wallet) Network() tron.Network {
	return w.cfg.Network
}

func (w *Wallet) JarAddress(ctx context.Context, index int64) (tron.Address, error) {
	return w.signer.JarAddress(ctx, index)
}

func (w *Wallet) PrepareSweep(ctx context.Context, p *wallet.Preparation) (wallet.Readiness, error) {
	resources, err := w.chain.AccountResources(ctx, p.Jar)
	if err != nil {
		return wallet.Readiness{}, err
	}

	if !resources.Exists || (resources.BandwidthFree < w.cfg.MinBandwidth && resources.BalanceSun < w.cfg.BandwidthReserveSun) {
		if !resources.Exists {
			return w.topUp(ctx, p, w.cfg.ActivationSun, w.cfg.AccountCreationSun, "activation", "activation")
		}
		return w.topUp(ctx, p, w.cfg.BandwidthReserveSun, 0, "bandwidth_reserve", "activation")
	}

	needed, err := w.chain.EstimateUSDTTransferEnergy(ctx, p.Jar, p.Destination, p.NetAmount)
	if err != nil {
		return wallet.Readiness{}, err
	}
	p.EnergyRequired = &needed
	energyFee, err := w.chain.EnergyFeeSun(ctx)
	if err != nil {
		return wallet.Readiness{}, err
	}
	burnSun := (needed*energyFee*11 + 9) / 10

	if resources.EnergyAvailable >= needed {
		feeLimit := min(max(burnSun/10, 1_000_000), w.cfg.MaxFeeLimitSun)
		p.FeeLimitSun = &feeLimit
		return wallet.Readiness{Ready: true, Note: fmt.Sprintf("%d energy available for %d needed", resources.EnergyAvailable, needed)}, nil
	}

	if !p.BurnFallback {
		ready, done, err := w.rent(ctx, p, needed)
		if done || err != nil {
			return ready, err
		}
	}

	if burnSun > w.cfg.MaxFeeLimitSun {
		return wallet.Readiness{}, fmt.Errorf("%w: burning %d sun exceeds the fee limit cap", wallet.ErrPermanent, burnSun)
	}
	required := burnSun + w.cfg.BandwidthReserveSun
	if resources.BalanceSun < required {
		return w.topUp(ctx, p, required-resources.BalanceSun, 0, "energy_burn", "burn_topup")
	}
	p.FeeLimitSun = &burnSun
	return wallet.Readiness{Ready: true, Note: fmt.Sprintf("burning up to %d sun for %d energy", burnSun, needed)}, nil
}

func (w *Wallet) rent(ctx context.Context, p *wallet.Preparation, needed int64) (wallet.Readiness, bool, error) {
	if w.renter == nil {
		p.BurnFallback = true
		return wallet.Readiness{}, false, nil
	}
	if p.EnergyOrderRef != nil && p.EnergyOrderedAt != nil {
		if w.now().Sub(*p.EnergyOrderedAt) < w.cfg.RentalTimeout {
			return wallet.Readiness{RetryAfter: w.cfg.PollInterval, Note: "waiting for rented energy to be delegated"}, true, nil
		}
		w.logger.Warn("rented energy did not arrive in time; falling back to burning TRX", "settlement_id", p.SettlementID, "order", *p.EnergyOrderRef)
		p.BurnFallback = true
		return wallet.Readiness{}, false, nil
	}
	amount := int64(math.Ceil(float64(needed)*w.cfg.EnergyMargin/1000)) * 1000
	rental, err := w.renter.Rent(ctx, p.Jar, amount)
	if err != nil {
		if ctx.Err() != nil {
			return wallet.Readiness{}, true, ctx.Err()
		}
		w.logger.Warn("energy rental not possible; falling back to burning TRX", "settlement_id", p.SettlementID, "provider", w.renter.Name(), "reason", err)
		p.BurnFallback = true
		return wallet.Readiness{}, false, nil
	}
	if rental.Amount > 0 {
		amount = rental.Amount
	}
	orderedAt := w.now()
	p.EnergyOrderRef, p.EnergyOrderedAt = &rental.Reference, &orderedAt
	p.Expenses = append(p.Expenses, wallet.Expense{
		Kind:      "energy_rental",
		TRXSun:    rental.TotalSun,
		Reference: rental.Provider + ":" + rental.Reference,
		Detail:    map[string]any{"provider": rental.Provider, "energy": amount, "unit_price_sun": rental.UnitPriceSun},
	})
	w.logger.Info("energy rented for sweep", "settlement_id", p.SettlementID, "provider", rental.Provider, "energy", amount,
		"cost_sun", rental.TotalSun, "unit_price_sun", rental.UnitPriceSun)
	return wallet.Readiness{RetryAfter: w.cfg.PollInterval, Note: "energy ordered"}, true, nil
}

func (w *Wallet) topUp(ctx context.Context, p *wallet.Preparation, amountSun, networkFeeSun int64, purpose, expenseKind string) (wallet.Readiness, error) {
	if p.TopUpSentAt != nil && w.now().Sub(*p.TopUpSentAt) < w.cfg.TopUpWait {
		return wallet.Readiness{RetryAfter: w.cfg.PollInterval, Note: "waiting for TRX top-up to land"}, nil
	}
	signed, err := w.signer.SignTRXTransfer(ctx, p.Jar, amountSun, purpose)
	if err != nil {
		return wallet.Readiness{}, fmt.Errorf("sign %s top-up: %w", purpose, err)
	}
	if err := w.chain.Broadcast(ctx, signed.Raw); err != nil {
		return wallet.Readiness{}, fmt.Errorf("broadcast %s top-up: %w", purpose, err)
	}
	sentAt := w.now()
	p.TopUpTxID, p.TopUpSentAt = &signed.TxID, &sentAt
	p.Expenses = append(p.Expenses, wallet.Expense{
		Kind:      expenseKind,
		TRXSun:    amountSun + networkFeeSun,
		Reference: signed.TxID,
		Detail:    map[string]any{"purpose": purpose, "sent_sun": amountSun, "network_fee_sun": networkFeeSun},
	})
	w.logger.Info("treasury topped up jar", "settlement_id", p.SettlementID, "purpose", purpose, "amount_sun", amountSun, "tx_id", signed.TxID)
	return wallet.Readiness{RetryAfter: w.cfg.PollInterval, Note: purpose + " sent"}, nil
}

func (w *Wallet) SignSweep(ctx context.Context, request wallet.SweepRequest) (wallet.SignedTx, error) {
	feeLimit := request.FeeLimitSun
	if feeLimit <= 0 {
		feeLimit = 1_000_000
	}
	signed, from, err := w.signer.SignUSDTTransfer(ctx, request.JarIndex, request.Destination, request.NetAmount, feeLimit)
	if err != nil {
		return wallet.SignedTx{}, err
	}
	if from != request.Jar {
		return wallet.SignedTx{}, fmt.Errorf("%w: %w: signed from %s, expected %s", wallet.ErrPermanent, errUnexpectedSigner, from, request.Jar)
	}
	return signed, nil
}

func (w *Wallet) Broadcast(ctx context.Context, tx wallet.SignedTx) error {
	return w.chain.Broadcast(ctx, tx.Raw)
}

func (w *Wallet) TransactionKnown(ctx context.Context, txID string) (bool, error) {
	return w.chain.TransactionKnown(ctx, txID)
}
