//go:build trongrid

package trongrid

import (
	"context"
	"os"
	"testing"
	"time"

	"gigjar/internal/chain"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

func liveNileClient(t *testing.T) *Client {
	t.Helper()
	key := os.Getenv("GIGJAR_TRONGRID_API_KEY")
	if key == "" {
		t.Fatal("GIGJAR_TRONGRID_API_KEY must be set for live TronGrid tests")
	}
	baseURL, _ := DefaultBaseURL(tron.NetworkNile)
	client, err := New(Config{BaseURL: baseURL, APIKey: key, Network: tron.NetworkNile, MaxPages: 2})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestLiveNileSolidifiedTransfer(t *testing.T) {
	client := liveNileClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	outcome, err := client.TransactionOutcome(ctx, solidTxHash)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != chain.OutcomeFinal {
		t.Fatalf("outcome = %+v", outcome)
	}
	transfers := outcome.TransfersTo(jarAddress)
	if len(transfers) != 1 || transfers[0].Amount != 1_000_000_000 {
		t.Fatalf("transfers = %+v", transfers)
	}

	listing, err := client.ListIncoming(ctx, jarAddress, time.UnixMilli(1794159243000))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, observation := range listing.Observations {
		if observation.TxHash == solidTxHash && observation.Amount == 1_000_000_000 {
			found = true
		}
	}
	if !found {
		t.Fatalf("listing did not include %s: %+v", solidTxHash, listing.Observations)
	}

	if balance, err := client.TokenBalance(ctx, jarAddress); err != nil || balance <= 0 {
		t.Fatalf("balance = %s, %v", balance, err)
	}
	unactivated, err := client.TokenBalance(ctx, "TCYMsvYnDwf96uHssU1X12onp3ju792esi")
	if err != nil || unactivated < usdt.MustParse("11") {
		t.Fatalf("an unactivated address holding USDT must report its balance: %s, %v", unactivated, err)
	}

	notFinal, err := client.TransactionOutcome(ctx, "0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil || notFinal.State != chain.OutcomeNotFinal {
		t.Fatalf("unknown tx outcome = %+v, %v", notFinal, err)
	}
}

func TestLiveNileResourceQueries(t *testing.T) {
	client := liveNileClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resources, err := client.AccountResources(ctx, jarAddress)
	if err != nil || !resources.Exists {
		t.Fatalf("resources = %+v, %v", resources, err)
	}
	missing, err := client.AccountResources(ctx, tron.SimulatedJarAddress(424242))
	if err != nil || missing.Exists {
		t.Fatalf("unactivated account = %+v, %v", missing, err)
	}
	energy, err := client.EstimateUSDTTransferEnergy(ctx, "TVF2Mp9QY7FEGTnr3DBpFLobA6jguHyMvi", jarAddress, 1_000_000)
	if err != nil || energy < 10_000 || energy > 200_000 {
		t.Fatalf("energy estimate = %d, %v", energy, err)
	}
	fee, err := client.EnergyFeeSun(ctx)
	if err != nil || fee <= 0 {
		t.Fatalf("energy fee = %d, %v", fee, err)
	}
	known, err := client.TransactionKnown(ctx, solidTxHash)
	if err != nil || !known {
		t.Fatalf("known solid tx = %v, %v", known, err)
	}
	unknown, err := client.TransactionKnown(ctx, "0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil || unknown {
		t.Fatalf("unknown tx = %v, %v", unknown, err)
	}
	t.Logf("nile: transfer to a holder needs %d energy at %d sun each", energy, fee)
}
