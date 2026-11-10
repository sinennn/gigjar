package tronsave

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"gigjar/internal/energy"
	"gigjar/internal/tron"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, maxPrice float64) (*Client, *[]int64) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	var payments []int64
	client, err := New(Config{
		BaseURL: server.URL, PaymentAddress: "TATT1UzHRikft98bRFqApFTsaSw73ycfoS", MaxUnitPriceSun: maxPrice,
		Pay: func(_ context.Context, _ tron.Address, amount int64) ([]byte, string, error) {
			payments = append(payments, amount)
			return []byte(`{"txID":"pay"}`), "pay", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client, &payments
}

func TestRentAcceptsCreatedStatus(t *testing.T) {
	client, payments := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/estimate-buy-resource":
			io.WriteString(w, `{"error":false,"message":"Success","data":{"unitPrice":65,"durationSec":900,"estimateTrx":4225000,"availableResource":65000}}`)
		case "/v2/buy-resource":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"error":false,"message":"Success","data":{"orderId":"abc123"}}`)
		}
	}, 90)
	rental, err := client.Rent(context.Background(), "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA", 31000)
	if err != nil || rental.Amount != MinimumOrder || rental.Reference != "abc123" || rental.TotalSun != 4225000 || len(*payments) != 1 || (*payments)[0] != 4225000 {
		t.Fatalf("rental = %+v, err = %v, payments = %v", rental, err, *payments)
	}
}

func TestRentRefusesExpensiveOrThinMarkets(t *testing.T) {
	for name, body := range map[string]string{
		"expensive": `{"error":false,"message":"Success","data":{"unitPrice":95,"durationSec":900,"estimateTrx":6175000,"availableResource":65000}}`,
		"thin":      `{"error":false,"message":"Success","data":{"unitPrice":65,"durationSec":900,"estimateTrx":4225000,"availableResource":1000}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client, payments := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) }, 90)
			_, err := client.Rent(context.Background(), "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA", 65000)
			if !errors.Is(err, energy.ErrTooExpensive) && !errors.Is(err, energy.ErrUnavailable) {
				t.Fatalf("err = %v", err)
			}
			if len(*payments) != 0 {
				t.Fatal("nothing may be paid when the order is refused")
			}
		})
	}
}

func TestBusinessErrorsAreErrors(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"error":true,"message":"TSAS:500 SOMETHING_BROKE","data":null}`)
	}, 90)
	if _, err := client.Quote(context.Background(), "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA", 65000); err == nil {
		t.Fatal("expected an error")
	}
}
