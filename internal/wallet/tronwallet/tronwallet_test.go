package tronwallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"gigjar/internal/chain/trongrid"
	"gigjar/internal/energy"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet"
)

const (
	jar  tron.Address = "TYrvxrBRLUrNnEEpF4uEmEq27CfxN5AtBy"
	dest tron.Address = "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA"
)

type fakeChain struct {
	resources  trongrid.AccountResources
	needed     int64
	broadcasts []json.RawMessage
}

func (f *fakeChain) AccountResources(context.Context, tron.Address) (trongrid.AccountResources, error) {
	return f.resources, nil
}
func (f *fakeChain) EstimateUSDTTransferEnergy(context.Context, tron.Address, tron.Address, usdt.Amount) (int64, error) {
	return f.needed, nil
}
func (f *fakeChain) EnergyFeeSun(context.Context) (int64, error) { return 100, nil }
func (f *fakeChain) Broadcast(_ context.Context, raw json.RawMessage) error {
	f.broadcasts = append(f.broadcasts, raw)
	return nil
}
func (f *fakeChain) TransactionKnown(context.Context, string) (bool, error) { return false, nil }

type fakeSigner struct {
	trx  []int64
	from tron.Address
}

func (f *fakeSigner) JarAddress(context.Context, int64) (tron.Address, error) { return jar, nil }
func (f *fakeSigner) SignUSDTTransfer(_ context.Context, _ int64, _ tron.Address, _ usdt.Amount, feeLimit int64) (wallet.SignedTx, tron.Address, error) {
	from := f.from
	if from == "" {
		from = jar
	}
	return wallet.SignedTx{TxID: fmt.Sprintf("%064d", feeLimit), Raw: json.RawMessage(`{}`), ExpiresAt: time.Now().Add(time.Minute)}, from, nil
}
func (f *fakeSigner) SignTRXTransfer(_ context.Context, _ tron.Address, amount int64, _ string) (wallet.SignedTx, error) {
	f.trx = append(f.trx, amount)
	return wallet.SignedTx{TxID: fmt.Sprintf("%064d", len(f.trx)), Raw: json.RawMessage(`{"trx":true}`)}, nil
}

type fakeRenter struct {
	err     error
	rentals []int64
}

func (f *fakeRenter) Name() string { return "fake" }
func (f *fakeRenter) Rent(_ context.Context, _ tron.Address, amount int64) (energy.Rental, error) {
	if f.err != nil {
		return energy.Rental{}, f.err
	}
	f.rentals = append(f.rentals, amount)
	return energy.Rental{Reference: fmt.Sprintf("order-%d", len(f.rentals)), TotalSun: amount * 65, UnitPriceSun: 65, Provider: "fake"}, nil
}

func newTestWallet(c *fakeChain, s *fakeSigner, r energy.Renter) (*Wallet, *time.Time) {
	clock := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	w := New(c, s, r, slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultConfig(tron.NetworkNile))
	w.now = func() time.Time { return clock }
	return w, &clock
}

func preparation() *wallet.Preparation {
	return &wallet.Preparation{SettlementID: uuid.New(), JarIndex: 1, Jar: jar, Destination: dest, NetAmount: usdt.MustParse("11")}
}

func TestNewJarIsActivatedOnceThenWaits(t *testing.T) {
	c := &fakeChain{needed: 130285}
	s := &fakeSigner{}
	w, clock := newTestWallet(c, s, &fakeRenter{})
	p := preparation()

	ready, err := w.PrepareSweep(context.Background(), p)
	if err != nil || ready.Ready || len(s.trx) != 1 || s.trx[0] != 100_000 || len(c.broadcasts) != 1 {
		t.Fatalf("ready=%+v err=%v trx=%v", ready, err, s.trx)
	}
	if len(p.Expenses) != 1 || p.Expenses[0].Kind != "activation" || p.Expenses[0].TRXSun != 1_200_000 || p.TopUpTxID == nil {
		t.Fatalf("expenses=%+v", p.Expenses)
	}
	*clock = clock.Add(30 * time.Second)
	if _, err := w.PrepareSweep(context.Background(), p); err != nil || len(s.trx) != 1 {
		t.Fatalf("must not top up twice while the first is landing: %v %v", s.trx, err)
	}
}

func TestEnoughEnergyIsReadyWithASmallFeeLimit(t *testing.T) {
	c := &fakeChain{needed: 64285, resources: trongrid.AccountResources{Exists: true, BalanceSun: 1_000_000, EnergyAvailable: 70000, BandwidthFree: 600}}
	w, _ := newTestWallet(c, &fakeSigner{}, &fakeRenter{})
	p := preparation()
	ready, err := w.PrepareSweep(context.Background(), p)
	if err != nil || !ready.Ready || p.FeeLimitSun == nil || *p.FeeLimitSun < 1_000_000 || *p.FeeLimitSun > 30_000_000 {
		t.Fatalf("ready=%+v err=%v fee=%v", ready, err, p.FeeLimitSun)
	}
	if *p.EnergyRequired != 64285 {
		t.Fatalf("energy required = %d", *p.EnergyRequired)
	}
}

func TestMissingEnergyIsRentedWithMarginThenUsed(t *testing.T) {
	c := &fakeChain{needed: 130285, resources: trongrid.AccountResources{Exists: true, BalanceSun: 1_000_000, BandwidthFree: 600}}
	r := &fakeRenter{}
	w, clock := newTestWallet(c, &fakeSigner{}, r)
	p := preparation()

	ready, err := w.PrepareSweep(context.Background(), p)
	if err != nil || ready.Ready || len(r.rentals) != 1 || r.rentals[0] != 137000 {
		t.Fatalf("ready=%+v err=%v rentals=%v", ready, err, r.rentals)
	}
	if p.EnergyOrderRef == nil || len(p.Expenses) != 1 || p.Expenses[0].Kind != "energy_rental" || p.Expenses[0].TRXSun != 137000*65 {
		t.Fatalf("preparation = %+v", p)
	}
	*clock = clock.Add(20 * time.Second)
	p.Expenses = nil
	if ready, _ := w.PrepareSweep(context.Background(), p); ready.Ready || len(r.rentals) != 1 {
		t.Fatal("must wait for delegation without renting again")
	}
	c.resources.EnergyAvailable = 137000
	ready, err = w.PrepareSweep(context.Background(), p)
	if err != nil || !ready.Ready || p.BurnFallback {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
}

func TestExpensiveRentalFallsBackToBurningTRX(t *testing.T) {
	c := &fakeChain{needed: 130285, resources: trongrid.AccountResources{Exists: true, BalanceSun: 1_000_000, BandwidthFree: 600}}
	s := &fakeSigner{}
	w, clock := newTestWallet(c, s, &fakeRenter{err: errors.Join(energy.ErrTooExpensive, errors.New("200 sun"))})
	p := preparation()

	ready, err := w.PrepareSweep(context.Background(), p)
	burn := int64(14331350)
	if err != nil || ready.Ready || !p.BurnFallback || len(s.trx) != 1 || s.trx[0] != burn+400_000-1_000_000 {
		t.Fatalf("ready=%+v err=%v trx=%v burn=%v", ready, err, s.trx, p.BurnFallback)
	}
	*clock = clock.Add(2 * time.Minute)
	c.resources.BalanceSun = burn + 400_000
	ready, err = w.PrepareSweep(context.Background(), p)
	if err != nil || !ready.Ready || *p.FeeLimitSun != burn {
		t.Fatalf("ready=%+v err=%v fee=%v", ready, err, p.FeeLimitSun)
	}
}

func TestRentalThatNeverArrivesFallsBackAfterTimeout(t *testing.T) {
	c := &fakeChain{needed: 64285, resources: trongrid.AccountResources{Exists: true, BalanceSun: 1_000_000, BandwidthFree: 600}}
	s := &fakeSigner{}
	w, clock := newTestWallet(c, s, &fakeRenter{})
	p := preparation()
	if _, err := w.PrepareSweep(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(4 * time.Minute)
	if _, err := w.PrepareSweep(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !p.BurnFallback || len(s.trx) != 1 {
		t.Fatalf("burn=%v trx=%v", p.BurnFallback, s.trx)
	}
}

func TestSignerSigningFromTheWrongAddressIsRejected(t *testing.T) {
	w, _ := newTestWallet(&fakeChain{}, &fakeSigner{from: dest}, nil)
	_, err := w.SignSweep(context.Background(), wallet.SweepRequest{JarIndex: 1, Jar: jar, Destination: dest, NetAmount: 1, FeeLimitSun: 1})
	if !errors.Is(err, wallet.ErrPermanent) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoRenterBurnsDirectly(t *testing.T) {
	c := &fakeChain{needed: 64285, resources: trongrid.AccountResources{Exists: true, BalanceSun: 50_000_000, BandwidthFree: 600}}
	w, _ := newTestWallet(c, &fakeSigner{}, nil)
	p := preparation()
	ready, err := w.PrepareSweep(context.Background(), p)
	if err != nil || !ready.Ready || !p.BurnFallback {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
}

func TestAnyRentalFailureFallsBackToBurning(t *testing.T) {
	c := &fakeChain{needed: 29650, resources: trongrid.AccountResources{Exists: true, BalanceSun: 100_000, BandwidthFree: 600}}
	s := &fakeSigner{}
	w, _ := newTestWallet(c, s, &fakeRenter{err: errors.New("TSAS:408 INVALID_PARAMS")})
	p := preparation()
	ready, err := w.PrepareSweep(context.Background(), p)
	if err != nil || ready.Ready || !p.BurnFallback || len(s.trx) != 1 {
		t.Fatalf("ready=%+v err=%v burn=%v trx=%v", ready, err, p.BurnFallback, s.trx)
	}
}
