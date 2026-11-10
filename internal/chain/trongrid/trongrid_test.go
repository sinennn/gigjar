package trongrid

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gigjar/internal/chain"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

const (
	jarAddress  tron.Address = "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA"
	solidTxHash              = "ce8eb3c878ce2d1a435c1990e39aa021da835c57a8547f404e499474b799c7a1"
	solidTxInfo              = `{"id": "ce8eb3c878ce2d1a435c1990e39aa021da835c57a8547f404e499474b799c7a1","blockNumber": 72540166,"blockTimeStamp": 1794159243000,"contractResult": ["0000000000000000000000000000000000000000000000000000000000000000"],"contract_address": "41eca9bc828a3005b9a3b909f2cc5c2a54794de05f","receipt": {"origin_energy_usage": 14650,"energy_usage_total": 14650,"net_usage": 346,"result": "SUCCESS"},"log": [{"address": "eca9bc828a3005b9a3b909f2cc5c2a54794de05f","topics": ["ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","000000000000000000000000d3682962027e721c5247a9faf7865fe4a71d5438","000000000000000000000000b5131044ccc9d9600aa4384157017dd83496511e"],"data": "000000000000000000000000000000000000000000000000000000003b9aca00"}]}`
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Network: tron.NetworkNile, PageSize: 2, MaxPages: 3})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestListIncomingFollowsPagesAndAggregatesPerTransaction(t *testing.T) {
	var calls int
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get(apiKeyHeader) != "test-key" {
			t.Errorf("missing API key header")
		}
		query := r.URL.Query()
		if query.Get("only_to") != "true" || query.Get("contract_address") != "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf" || query.Get("min_timestamp") != "1794000000000" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		if query.Get("fingerprint") == "" {
			io.WriteString(w, `{"success":true,"data":[
				{"transaction_id":"954fc8e237b813eea05121c550588a87dda1702808836dabe755838c7df0d179","token_info":{"symbol":"USDT","address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","decimals":6},"block_timestamp":1794153696000,"from":"TCRF6D4tBH8pcYxYiLG6vX8e7rNZ96CNk5","to":"TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA","type":"Transfer","value":"500000000"},
				{"transaction_id":"954fc8e237b813eea05121c550588a87dda1702808836dabe755838c7df0d179","token_info":{"symbol":"USDT","address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","decimals":6},"block_timestamp":1794153696000,"from":"TCRF6D4tBH8pcYxYiLG6vX8e7rNZ96CNk5","to":"TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA","type":"Transfer","value":"1"}
			],"meta":{"fingerprint":"next-page"}}`)
			return
		}
		io.WriteString(w, `{"success":true,"data":[
			{"transaction_id":"ce8eb3c878ce2d1a435c1990e39aa021da835c57a8547f404e499474b799c7a1","token_info":{"symbol":"USDT","address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","decimals":6},"block_timestamp":1794159243000,"from":"TVF2Mp9QY7FEGTnr3DBpFLobA6jguHyMvi","to":"TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA","type":"Transfer","value":"1000000000"},
			{"transaction_id":"aa8eb3c878ce2d1a435c1990e39aa021da835c57a8547f404e499474b799c7a1","token_info":{"symbol":"USDT","address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf","decimals":6},"block_timestamp":1794159244000,"from":"TVF2Mp9QY7FEGTnr3DBpFLobA6jguHyMvi","to":"TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA","type":"Transfer","value":"0"},
			{"transaction_id":"bb8eb3c878ce2d1a435c1990e39aa021da835c57a8547f404e499474b799c7a1","token_info":{"symbol":"FAKE","address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","decimals":6},"block_timestamp":1794159245000,"from":"TVF2Mp9QY7FEGTnr3DBpFLobA6jguHyMvi","to":"TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA","type":"Transfer","value":"5000000"}
		],"meta":{}}`)
	})

	listing, err := client.ListIncoming(context.Background(), jarAddress, time.UnixMilli(1794000000000))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !listing.Complete {
		t.Fatalf("calls = %d complete = %v", calls, listing.Complete)
	}
	if len(listing.Observations) != 2 {
		t.Fatalf("observations = %+v", listing.Observations)
	}
	if listing.Observations[0].Amount != usdt.MustParse("500.000001") {
		t.Errorf("aggregated amount = %s", listing.Observations[0].Amount)
	}
	if listing.Observations[1].TxHash != solidTxHash || listing.Observations[1].Amount != usdt.MustParse("1000") {
		t.Errorf("second observation = %+v", listing.Observations[1])
	}
	if !listing.NewestTimestamp.Equal(time.UnixMilli(1794159243000)) {
		t.Errorf("newest = %s", listing.NewestTimestamp)
	}
}

func TestListIncomingStopsAtPageBudget(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":true,"data":[{"transaction_id":"954fc8e237b813eea05121c550588a87dda1702808836dabe755838c7df0d179","token_info":{"address":"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"},"block_timestamp":1794153696000,"from":"TCRF6D4tBH8pcYxYiLG6vX8e7rNZ96CNk5","to":"TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA","type":"Transfer","value":"5"}],"meta":{"fingerprint":"more"}}`)
	})
	listing, err := client.ListIncoming(context.Background(), jarAddress, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if listing.Complete {
		t.Fatal("listing must report more pages remain")
	}
}

func TestTransactionOutcomeParsesSolidifiedTransfer(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/walletsolidity/gettransactioninfobyid" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), solidTxHash) {
			t.Errorf("body = %s", body)
		}
		io.WriteString(w, solidTxInfo)
	})
	outcome, err := client.TransactionOutcome(context.Background(), strings.ToUpper(solidTxHash))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.State != chain.OutcomeFinal || outcome.BlockNumber != 72540166 {
		t.Fatalf("outcome = %+v", outcome)
	}
	transfers := outcome.TransfersTo(jarAddress)
	if len(transfers) != 1 || transfers[0].Amount != usdt.MustParse("1000") || transfers[0].LogIndex != 0 {
		t.Fatalf("transfers = %+v", transfers)
	}
	if transfers[0].From != "TVF2Mp9QY7FEGTnr3DBpFLobA6jguHyMvi" {
		t.Errorf("from = %s", transfers[0].From)
	}
}

func TestTransactionOutcomeStates(t *testing.T) {
	cases := map[string]struct {
		body   string
		state  chain.OutcomeState
		reason string
	}{
		"not solidified": {`{}`, chain.OutcomeNotFinal, ""},
		"reverted":       {`{"id":"` + solidTxHash + `","blockNumber":1,"blockTimeStamp":1794159243000,"result":"FAILED","receipt":{"result":"REVERT"}}`, chain.OutcomeFailed, "receipt_revert"},
		"out of energy":  {`{"id":"` + solidTxHash + `","blockNumber":1,"blockTimeStamp":1794159243000,"receipt":{"result":"OUT_OF_ENERGY"}}`, chain.OutcomeFailed, "receipt_out_of_energy"},
		"foreign token":  {strings.ReplaceAll(solidTxInfo, `"address": "eca9bc`, `"address": "a614f8`), chain.OutcomeFinal, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, c.body) })
			outcome, err := client.TransactionOutcome(context.Background(), solidTxHash)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.State != c.state || outcome.FailureReason != c.reason {
				t.Fatalf("outcome = %+v", outcome)
			}
			if name == "foreign token" && len(outcome.Transfers) != 0 {
				t.Fatalf("transfers of another token must be ignored: %+v", outcome.Transfers)
			}
		})
	}
}

func TestTransactionOutcomeRejectsMismatchedInfo(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, strings.Replace(solidTxInfo, `"id": "ce8e`, `"id": "ff8e`, 1))
	})
	if _, err := client.TransactionOutcome(context.Background(), solidTxHash); !errors.Is(err, chain.ErrMalformed) {
		t.Fatalf("err = %v, want ErrMalformed", err)
	}
}

func TestHTTPErrorsAreClassified(t *testing.T) {
	cases := map[int]error{http.StatusTooManyRequests: chain.ErrRateLimited, http.StatusBadGateway: chain.ErrUnavailable}
	for status, want := range cases {
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		if _, err := client.TokenBalance(context.Background(), jarAddress); !errors.Is(err, want) {
			t.Errorf("status %d: err = %v, want %v", status, err, want)
		}
	}
}

func TestErrorsNeverContainTheAPIKey(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"Error":"bad"}`)
	})
	_, err := client.TokenBalance(context.Background(), jarAddress)
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("err = %v", err)
	}
}

func TestTokenBalance(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"success":true,"data":[{"trc20":[{"TTvwmPCRUg8ut6GNA2NiATsZATFjx1t4TC":"250000000"},{"TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf":"2490000000"}]}]}`)
	})
	balance, err := client.TokenBalance(context.Background(), jarAddress)
	if err != nil || balance != usdt.MustParse("2490") {
		t.Fatalf("balance = %s, %v", balance, err)
	}

	empty := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"success":true,"data":[]}`) })
	balance, err = empty.TokenBalance(context.Background(), jarAddress)
	if err != nil || balance != 0 {
		t.Fatalf("unactivated balance = %s, %v", balance, err)
	}
}

func TestNewRefusesSimulatedNetwork(t *testing.T) {
	if _, err := New(Config{BaseURL: "https://example.com", Network: tron.NetworkSimulated}); err == nil {
		t.Fatal("expected error")
	}
}
