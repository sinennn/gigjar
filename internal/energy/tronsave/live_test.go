//go:build trongrid

package tronsave_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gigjar/internal/chain/trongrid"
	"gigjar/internal/energy/tronsave"
	"gigjar/internal/tron"
	"gigjar/internal/wallet/tronwallet"
)

func TestLiveNileActivationAndEnergyRental(t *testing.T) {
	key, token, signerURL := os.Getenv("GIGJAR_TRONGRID_API_KEY"), os.Getenv("GIGJAR_SIGNER_TOKEN"), os.Getenv("GIGJAR_SIGNER_URL")
	if key == "" || token == "" || signerURL == "" {
		t.Fatal("GIGJAR_TRONGRID_API_KEY, GIGJAR_SIGNER_TOKEN and GIGJAR_SIGNER_URL must be set")
	}
	ctx := context.Background()
	baseURL, _ := trongrid.DefaultBaseURL(tron.NetworkNile)
	grid, err := trongrid.New(trongrid.Config{BaseURL: baseURL, APIKey: key, Network: tron.NetworkNile})
	if err != nil {
		t.Fatal(err)
	}
	signer := tronwallet.NewSignerClient(signerURL, token)
	treasury, err := signer.TreasuryAddress(ctx)
	if err != nil {
		t.Fatalf("signer unreachable: %v", err)
	}
	before, err := grid.AccountResources(ctx, treasury)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("treasury %s: %.6f TRX", treasury, float64(before.BalanceSun)/1e6)

	jarIndex := time.Now().Unix()
	jar, err := signer.JarAddress(ctx, jarIndex)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := grid.AccountResources(ctx, jar)
	if err != nil || fresh.Exists {
		t.Fatalf("expected an unactivated fresh jar %s: %+v %v", jar, fresh, err)
	}
	t.Logf("fresh jar %s (index %d)", jar, jarIndex)

	activation, err := signer.SignTRXTransfer(ctx, jar, 100_000, "activation")
	if err != nil {
		t.Fatal(err)
	}
	if err := grid.Broadcast(ctx, activation.Raw); err != nil {
		t.Fatal(err)
	}
	if err := grid.Broadcast(ctx, activation.Raw); err != nil {
		t.Fatalf("rebroadcasting the same signed transaction must be harmless: %v", err)
	}
	t.Logf("activation tx %s (expires %s)", activation.TxID, activation.ExpiresAt.Format(time.RFC3339))
	waitFor(t, 2*time.Minute, func() bool {
		r, err := grid.AccountResources(ctx, jar)
		return err == nil && r.Exists && r.BalanceSun == 100_000
	})
	activated, _ := grid.AccountResources(ctx, jar)
	t.Logf("jar activated: %.6f TRX, %d free bandwidth", float64(activated.BalanceSun)/1e6, activated.BandwidthFree)

	afterActivation, _ := grid.AccountResources(ctx, treasury)
	t.Logf("activation cost the treasury %.6f TRX (0.1 TRX sent to the jar)", float64(before.BalanceSun-afterActivation.BalanceSun)/1e6)

	renterURL, paymentAddress, _ := tronsave.Defaults(tron.NetworkNile)
	renter, err := tronsave.New(tronsave.Config{
		BaseURL: renterURL, PaymentAddress: paymentAddress, DurationSeconds: 900, MaxUnitPriceSun: 99,
		Pay: func(ctx context.Context, to tron.Address, amountSun int64) ([]byte, string, error) {
			signed, err := signer.SignTRXTransfer(ctx, to, amountSun, "energy_rental")
			if err == nil {
				t.Logf("payment to tronsave %s: %.6f TRX, tx %s", to, float64(amountSun)/1e6, signed.TxID)
			}
			return signed.Raw, signed.TxID, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := renter.Quote(ctx, jar, 65000)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tronsave nile quote for 65000 energy / 15 min: %.2f sun each, %.6f TRX total, %d available", quote.UnitPriceSun, float64(quote.TotalSun)/1e6, quote.Available)

	ordered := time.Now()
	rental, err := renter.Rent(ctx, jar, 65000)
	if err != nil {
		t.Fatalf("rent: %v", err)
	}
	t.Logf("tronsave order %s: %.6f TRX at %.2f sun", rental.Reference, float64(rental.TotalSun)/1e6, rental.UnitPriceSun)
	waitFor(t, 5*time.Minute, func() bool {
		r, err := grid.AccountResources(ctx, jar)
		return err == nil && r.EnergyAvailable >= 65000
	})
	delegated, _ := grid.AccountResources(ctx, jar)
	t.Logf("energy delegated to the jar after %s: %d available", time.Since(ordered).Round(time.Second), delegated.EnergyAvailable)

	after, _ := grid.AccountResources(ctx, treasury)
	t.Logf("treasury spent %.6f TRX in total for activation + rental", float64(before.BalanceSun-after.BalanceSun)/1e6)
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("condition not met within %s", timeout)
}
