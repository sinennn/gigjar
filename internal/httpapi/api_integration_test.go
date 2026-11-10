//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/chain/simulated"
	"gigjar/internal/httpapi"
	"gigjar/internal/identity"
	"gigjar/internal/jars"
	"gigjar/internal/paylinks"
	"gigjar/internal/settlement"
	"gigjar/internal/testdb"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet/simwallet"
	"gigjar/internal/watcher"
)

const clevaAddress = "TSUe9LcY9ny9AXnR46ZSHxYiSE6neyhRXA"

type client struct {
	t      *testing.T
	server *httptest.Server
	http   *http.Client
}

func startAPI(t *testing.T) (*pgxpool.Pool, *httptest.Server) {
	t.Helper()
	pool := testdb.Open(t)
	identityService, err := identity.NewService(pool, identity.Config{
		PasswordParams: identity.PasswordParams{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32},
		SessionTTL:     time.Hour,
		Network:        tron.NetworkSimulated,
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := httpapi.NewRouter(httpapi.RouterConfig{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
		RequestTimeout: 10 * time.Second,
		Readiness:      func(context.Context) error { return nil },
		DB:             pool,
		Identity:       identityService,
		Jars:           jars.NewService(pool, usdt.MustParse("12"), usdt.MustParse("1")),
		PayLinks:       paylinks.NewService(pool),
		Network:        tron.NetworkSimulated,
		PublicWebURL:   "http://localhost:3000",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return pool, server
}

func newClient(t *testing.T, server *httptest.Server) *client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &client{t: t, server: server, http: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any, headers map[string]string) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, c.server.URL+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := c.http.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	decoded := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			c.t.Fatalf("%s %s returned non-JSON body %q", method, path, raw)
		}
	}
	return response.StatusCode, decoded
}

func (c *client) expect(want int, method, path string, body any) map[string]any {
	c.t.Helper()
	status, decoded := c.do(method, path, body, nil)
	if status != want {
		c.t.Fatalf("%s %s = %d %v, want %d", method, path, status, decoded, want)
	}
	return decoded
}

func (c *client) register(email string) string {
	c.t.Helper()
	user := c.expect(http.StatusCreated, http.MethodPost, "/v1/auth/register", map[string]string{
		"email": email, "password": "long enough password", "display_name": "Ada",
	})
	return user["id"].(string)
}

func errorCode(body map[string]any) string {
	detail, _ := body["error"].(map[string]any)
	code, _ := detail["code"].(string)
	return code
}

func TestStudentJourney(t *testing.T) {
	pool, server := startAPI(t)
	student := newClient(t, server)
	userID := student.register("ada@uniuyo.edu.ng")

	me := student.expect(http.StatusOK, http.MethodGet, "/v1/me", nil)
	if me["email"] != "ada@uniuyo.edu.ng" || me["display_name"] != "Ada" {
		t.Fatalf("me = %v", me)
	}

	jar := student.expect(http.StatusOK, http.MethodGet, "/v1/jar", nil)
	jarAddress, _ := jar["address"].(string)
	if jar["network"] != "simulated" || len(jarAddress) != 34 || jar["cleva_destination_set"] != false {
		t.Fatalf("fresh jar = %v", jar)
	}
	if balances := jar["balances"].(map[string]any); balances["pending"] != "0.000000" {
		t.Fatalf("fresh jar balances = %v", balances)
	}
	if jar["settlement_threshold"] != "12.000000" || jar["settlement_fee"] != "1.000000" || jar["remaining_until_settlement"] != "12.000000" {
		t.Fatalf("fresh jar thresholds = %v", jar)
	}

	student.expect(http.StatusNotFound, http.MethodGet, "/v1/wallet/cleva", nil)
	invalid := student.expect(http.StatusUnprocessableEntity, http.MethodPut, "/v1/wallet/cleva", map[string]string{"address": "0xdAC17F958D2ee523a2206206994597C13D831ec7"})
	if errorCode(invalid) != "validation_failed" {
		t.Fatalf("invalid destination = %v", invalid)
	}
	for _, contract := range []string{"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t", "TXYZopYRdj2D9XRtbG411XZZ3kM5VkAeBf"} {
		if errorCode(student.expect(http.StatusUnprocessableEntity, http.MethodPut, "/v1/wallet/cleva", map[string]string{"address": contract})) != "validation_failed" {
			t.Fatalf("USDT contract %s accepted as a destination", contract)
		}
	}
	student.expect(http.StatusOK, http.MethodPut, "/v1/wallet/cleva", map[string]string{"address": clevaAddress})
	destination := student.expect(http.StatusOK, http.MethodGet, "/v1/wallet/cleva", nil)
	if destination["address"] != clevaAddress {
		t.Fatalf("destination = %v", destination)
	}

	idempotency := map[string]string{"Idempotency-Key": "flyer-1"}
	linkBody := map[string]string{"title": "Flyer design", "description": "A5 event flyer", "amount": "4"}
	status, link := student.do(http.MethodPost, "/v1/payment-links", linkBody, idempotency)
	if status != http.StatusCreated || link["amount"] != "4.000000" || link["status"] != "open" {
		t.Fatalf("create link = %d %v", status, link)
	}
	status, replay := student.do(http.MethodPost, "/v1/payment-links", linkBody, idempotency)
	if status != http.StatusOK || replay["id"] != link["id"] {
		t.Fatalf("idempotent replay = %d %v", status, replay)
	}
	status, conflict := student.do(http.MethodPost, "/v1/payment-links", map[string]string{"title": "Flyer design", "amount": "5"}, idempotency)
	if status != http.StatusConflict || errorCode(conflict) != "idempotency_key_conflict" {
		t.Fatalf("conflicting replay = %d %v", status, conflict)
	}
	reference := link["reference"].(string)
	if link["pay_url"] != "http://localhost:3000/pay/"+reference {
		t.Fatalf("pay url = %v", link["pay_url"])
	}

	badAmount := student.expect(http.StatusUnprocessableEntity, http.MethodPost, "/v1/payment-links", map[string]string{"title": "x", "amount": "4.1234567"})
	if errorCode(badAmount) != "validation_failed" {
		t.Fatalf("bad amount = %v", badAmount)
	}
	student.expect(http.StatusUnprocessableEntity, http.MethodPost, "/v1/payment-links", map[string]string{"title": "x", "amount": "0"})
	student.expect(http.StatusBadRequest, http.MethodPost, "/v1/payment-links", map[string]any{"title": "x", "amount": 4})

	for index := 0; index < 3; index++ {
		student.expect(http.StatusCreated, http.MethodPost, "/v1/payment-links", map[string]string{"title": fmt.Sprintf("Tutoring %d", index), "amount": "3"})
	}
	firstPage := student.expect(http.StatusOK, http.MethodGet, "/v1/payment-links?limit=3", nil)
	if items := firstPage["items"].([]any); len(items) != 3 || firstPage["next_cursor"] == nil {
		t.Fatalf("first page = %v", firstPage)
	}
	secondPage := student.expect(http.StatusOK, http.MethodGet, "/v1/payment-links?limit=3&cursor="+firstPage["next_cursor"].(string), nil)
	items := secondPage["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != link["id"] || secondPage["next_cursor"] != nil {
		t.Fatalf("second page = %v", secondPage)
	}

	public := newClient(t, server)
	instructions := public.expect(http.StatusOK, http.MethodGet, "/v1/public/pay/"+reference, nil)
	token := instructions["token"].(map[string]any)
	if instructions["pending_payment_observed"] != false || instructions["amount"] != "4.000000" || instructions["recipient_display_name"] != "Ada" ||
		instructions["pay_to_address"] != jarAddress || token["standard"] != "TRC20" || token["contract_address"] != nil {
		t.Fatalf("public instructions = %v", instructions)
	}
	if _, leaked := instructions["email"]; leaked {
		t.Fatal("public instructions leak the student's email")
	}
	public.expect(http.StatusNotFound, http.MethodGet, "/v1/public/pay/doesnotexist", nil)

	chainWatcher := watcher.New(pool, simulated.NewProvider(pool), slog.New(slog.NewTextHandler(io.Discard, nil)), watcher.DefaultConfig())
	runWatcher := func() {
		t.Helper()
		if _, err := pool.Exec(context.Background(), `UPDATE jar_watch_state SET next_poll_at = now()`); err != nil {
			t.Fatal(err)
		}
		if _, err := chainWatcher.DiscoverOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), `UPDATE incoming_transfers SET next_check_at = now() WHERE status = 'pending'`); err != nil {
			t.Fatal(err)
		}
		if _, err := chainWatcher.ConfirmOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	sent := public.expect(http.StatusCreated, http.MethodPost, "/v1/simulator/transfers",
		map[string]any{"reference": reference, "amount": "4", "confirmation_delay_seconds": 3600})
	if sent["simulated"] != true || sent["to_address"] != jarAddress {
		t.Fatalf("simulated transfer = %v", sent)
	}
	runWatcher()
	jar = student.expect(http.StatusOK, http.MethodGet, "/v1/jar", nil)
	if balances := jar["balances"].(map[string]any); balances["pending"] != "4.000000" || balances["available"] != "0.000000" {
		t.Fatalf("jar with pending payment = %v", balances)
	}
	incoming := student.expect(http.StatusOK, http.MethodGet, "/v1/incoming-transfers", nil)
	if items := incoming["items"].([]any); len(items) != 1 || items[0].(map[string]any)["status"] != "pending" || items[0].(map[string]any)["tx_hash"] != sent["tx_hash"] {
		t.Fatalf("incoming transfers while pending = %v", incoming)
	}
	if pendingPage := public.expect(http.StatusOK, http.MethodGet, "/v1/public/pay/"+reference, nil); pendingPage["pending_payment_observed"] != true || pendingPage["status"] != "open" {
		t.Fatalf("public page while pending = %v", pendingPage)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE simulated_chain_transfers SET solidifies_at = created_at`); err != nil {
		t.Fatal(err)
	}
	for _, amount := range []string{"3", "5"} {
		public.expect(http.StatusCreated, http.MethodPost, "/v1/simulator/transfers",
			map[string]any{"to_address": jarAddress, "amount": amount, "confirmation_delay_seconds": 0})
	}
	runWatcher()

	jar = student.expect(http.StatusOK, http.MethodGet, "/v1/jar", nil)
	balances := jar["balances"].(map[string]any)
	if balances["available"] != "12.000000" || balances["pending"] != "0.000000" || jar["remaining_until_settlement"] != "0.000000" || jar["cleva_destination_set"] != true {
		t.Fatalf("funded jar = %v", jar)
	}
	paid := student.expect(http.StatusOK, http.MethodGet, "/v1/payment-links/"+link["id"].(string), nil)
	linkTransfers := paid["transfers"].([]any)
	if paid["status"] != "paid" || paid["amount_received"] != "4.000000" || paid["paid_at"] == nil || len(linkTransfers) != 1 {
		t.Fatalf("paid link = %v", paid)
	}
	if linkTransfers[0].(map[string]any)["status"] != "confirmed" || linkTransfers[0].(map[string]any)["tx_hash"] != sent["tx_hash"] {
		t.Fatalf("paid link transfers = %v", linkTransfers)
	}
	if public.expect(http.StatusOK, http.MethodGet, "/v1/public/pay/"+reference, nil)["status"] != "paid" {
		t.Fatal("public page must show the link as paid")
	}
	incoming = student.expect(http.StatusOK, http.MethodGet, "/v1/incoming-transfers?limit=2", nil)
	if items := incoming["items"].([]any); len(items) != 2 || incoming["next_cursor"] == nil {
		t.Fatalf("incoming transfers page = %v", incoming)
	}
	rest := student.expect(http.StatusOK, http.MethodGet, "/v1/incoming-transfers?limit=2&cursor="+incoming["next_cursor"].(string), nil)
	if items := rest["items"].([]any); len(items) != 1 || rest["next_cursor"] != nil {
		t.Fatalf("incoming transfers second page = %v", rest)
	}
	student.expect(http.StatusUnprocessableEntity, http.MethodGet, "/v1/incoming-transfers?cursor=garbage", nil)

	transactions := student.expect(http.StatusOK, http.MethodGet, "/v1/transactions?limit=2", nil)
	movements := transactions["items"].([]any)
	if len(movements) != 2 || movements[0].(map[string]any)["kind"] != "deposit" || movements[0].(map[string]any)["incoming_transfer_id"] == nil || transactions["next_cursor"] == nil {
		t.Fatalf("transactions = %v", transactions)
	}

	if empty := student.expect(http.StatusOK, http.MethodGet, "/v1/settlements", nil); len(empty["items"].([]any)) != 0 {
		t.Fatalf("settlements before sweep = %v", empty)
	}
	sweeper, err := settlement.New(pool, simwallet.New(pool, 0), simulated.NewProvider(pool), slog.New(slog.NewTextHandler(io.Discard, nil)), settlement.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sweeper.QueueOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	settling := student.expect(http.StatusOK, http.MethodGet, "/v1/jar", nil)["balances"].(map[string]any)
	if settling["available"] != "0.000000" || settling["settling"] != "12.000000" {
		t.Fatalf("jar while settling = %v", settling)
	}
	queued := student.expect(http.StatusOK, http.MethodGet, "/v1/settlements", nil)["items"].([]any)[0].(map[string]any)
	if queued["state"] != "queued" || queued["amount"] != "12.000000" || queued["fee"] != "1.000000" || queued["net_amount"] != "11.000000" ||
		queued["destination_address"] != clevaAddress || queued["tx_id"] != nil {
		t.Fatalf("queued settlement = %v", queued)
	}
	for range 2 {
		if _, err := pool.Exec(context.Background(), `UPDATE settlements SET next_attempt_at = now()`); err != nil {
			t.Fatal(err)
		}
		if _, err := sweeper.AdvanceOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	confirmed := student.expect(http.StatusOK, http.MethodGet, "/v1/settlements", nil)["items"].([]any)[0].(map[string]any)
	if confirmed["state"] != "confirmed" || confirmed["tx_id"] == nil || confirmed["confirmed_at"] == nil {
		t.Fatalf("confirmed settlement = %v", confirmed)
	}
	settled := student.expect(http.StatusOK, http.MethodGet, "/v1/jar", nil)["balances"].(map[string]any)
	if settled["available"] != "0.000000" || settled["settling"] != "0.000000" || settled["settled"] != "11.000000" {
		t.Fatalf("jar after sweep = %v", settled)
	}
	history := student.expect(http.StatusOK, http.MethodGet, "/v1/transactions?limit=2", nil)["items"].([]any)
	if history[0].(map[string]any)["kind"] != "settlement_completed" || history[0].(map[string]any)["settled_delta"] != "11.000000" || history[0].(map[string]any)["settling_delta"] != "-12.000000" {
		t.Fatalf("sweep history = %v", history)
	}
	student.expect(http.StatusUnprocessableEntity, http.MethodGet, "/v1/settlements?cursor=garbage", nil)

	if _, err := pool.Exec(context.Background(), `UPDATE jars SET address = NULL, provisioned_at = NULL WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	notReady := public.expect(http.StatusServiceUnavailable, http.MethodGet, "/v1/public/pay/"+reference, nil)
	if errorCode(notReady) != "jar_not_ready" {
		t.Fatalf("unprovisioned jar = %v", notReady)
	}

	closed := student.expect(http.StatusOK, http.MethodPost, "/v1/payment-links/"+link["id"].(string)+"/close", nil)
	if closed["status"] != "closed" {
		t.Fatalf("closed link = %v", closed)
	}
	student.expect(http.StatusOK, http.MethodPost, "/v1/payment-links/"+link["id"].(string)+"/close", nil)

	student.expect(http.StatusNoContent, http.MethodPost, "/v1/auth/logout", nil)
	student.expect(http.StatusUnauthorized, http.MethodGet, "/v1/me", nil)
}

func TestStudentsCannotSeeEachOthersData(t *testing.T) {
	_, server := startAPI(t)
	ada := newClient(t, server)
	ada.register("ada@uniuyo.edu.ng")
	link := ada.expect(http.StatusCreated, http.MethodPost, "/v1/payment-links", map[string]string{"title": "Logo", "amount": "5"})

	bola := newClient(t, server)
	bola.register("bola@uniuyo.edu.ng")
	bola.expect(http.StatusNotFound, http.MethodGet, "/v1/payment-links/"+link["id"].(string), nil)
	bola.expect(http.StatusNotFound, http.MethodPost, "/v1/payment-links/"+link["id"].(string)+"/close", nil)
	list := bola.expect(http.StatusOK, http.MethodGet, "/v1/payment-links", nil)
	if len(list["items"].([]any)) != 0 {
		t.Fatalf("bola sees ada's links: %v", list)
	}
	still := ada.expect(http.StatusOK, http.MethodGet, "/v1/payment-links/"+link["id"].(string), nil)
	if still["status"] != "open" {
		t.Fatalf("ada's link changed: %v", still)
	}
}

func TestAuthenticationFailures(t *testing.T) {
	_, server := startAPI(t)
	ada := newClient(t, server)
	ada.register("ada@uniuyo.edu.ng")

	duplicate := newClient(t, server).expect(http.StatusConflict, http.MethodPost, "/v1/auth/register", map[string]string{
		"email": "ADA@uniuyo.edu.ng", "password": "long enough password", "display_name": "Ada again",
	})
	if errorCode(duplicate) != "email_taken" {
		t.Fatalf("duplicate = %v", duplicate)
	}

	weak := newClient(t, server).expect(http.StatusUnprocessableEntity, http.MethodPost, "/v1/auth/register", map[string]string{
		"email": "not-an-email", "password": "short", "display_name": "",
	})
	fields := weak["error"].(map[string]any)["fields"].(map[string]any)
	for _, field := range []string{"email", "password", "display_name"} {
		if fields[field] == nil {
			t.Errorf("missing field error for %s: %v", field, fields)
		}
	}

	attacker := newClient(t, server)
	attacker.expect(http.StatusUnauthorized, http.MethodPost, "/v1/auth/login", map[string]string{"email": "nobody@uniuyo.edu.ng", "password": "whatever password"})
	for attempt := 0; attempt < 10; attempt++ {
		status, _ := attacker.do(http.MethodPost, "/v1/auth/login", map[string]string{"email": "ada@uniuyo.edu.ng", "password": "wrong password!"}, nil)
		if status != http.StatusUnauthorized && status != http.StatusTooManyRequests {
			t.Fatalf("attempt %d status %d", attempt, status)
		}
	}
	limited := attacker.expect(http.StatusTooManyRequests, http.MethodPost, "/v1/auth/login", map[string]string{"email": "ada@uniuyo.edu.ng", "password": "long enough password"})
	if errorCode(limited) != "too_many_attempts" {
		t.Fatalf("rate limit = %v", limited)
	}

	fresh := newClient(t, server)
	fresh.expect(http.StatusUnauthorized, http.MethodPost, "/v1/auth/login", map[string]string{"email": "bola@uniuyo.edu.ng", "password": "long enough password"})

	status, _ := fresh.do(http.MethodGet, "/v1/me", nil, map[string]string{"Cookie": "gigjar_session=forged"})
	if status != http.StatusUnauthorized {
		t.Fatalf("forged session status = %d", status)
	}
}

func TestLoginIssuesWorkingSession(t *testing.T) {
	_, server := startAPI(t)
	newClient(t, server).register("ada@uniuyo.edu.ng")

	device := newClient(t, server)
	device.expect(http.StatusOK, http.MethodPost, "/v1/auth/login", map[string]string{"email": " Ada@UNIUYO.edu.ng ", "password": "long enough password"})
	renamed := device.expect(http.StatusOK, http.MethodPatch, "/v1/me", map[string]string{"display_name": "  Ada L.  "})
	if renamed["display_name"] != "Ada L." {
		t.Fatalf("renamed = %v", renamed)
	}
	device.expect(http.StatusUnprocessableEntity, http.MethodPatch, "/v1/me", map[string]string{"display_name": "   "})
}
