//go:build trongrid

package trongrid

import (
	"context"
	"os"
	"testing"
	"time"

	"gigjar/internal/chain"
	"gigjar/internal/tron"
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

	if _, err := client.TokenBalance(ctx, jarAddress); err != nil {
		t.Fatal(err)
	}

	notFinal, err := client.TransactionOutcome(ctx, "0000000000000000000000000000000000000000000000000000000000000001")
	if err != nil || notFinal.State != chain.OutcomeNotFinal {
		t.Fatalf("unknown tx outcome = %+v, %v", notFinal, err)
	}
}
