package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestRouter(readiness ReadinessProbe) http.Handler {
	router, err := NewRouter(RouterConfig{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedOrigins: []string{"http://localhost:3000"},
		RequestTimeout: time.Second,
		Readiness:      readiness,
	})
	if err != nil {
		panic(err)
	}
	return router
}

func serve(handler http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var body T
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body %q: %v", recorder.Body.String(), err)
	}
	return body
}

func TestLivenessIsIndependentOfDependencies(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return errors.New("database down") })

	recorder := serve(router, http.MethodGet, "/healthz", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if body := decodeBody[healthResponse](t, recorder); body.Status != "ok" {
		t.Errorf("status body = %q", body.Status)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("missing no-store cache header")
	}
}

func TestReadinessReflectsProbe(t *testing.T) {
	ready := serve(newTestRouter(func(context.Context) error { return nil }), http.MethodGet, "/readyz", nil)
	if ready.Code != http.StatusOK {
		t.Errorf("ready status = %d, want 200", ready.Code)
	}

	notReady := serve(newTestRouter(func(context.Context) error { return errors.New("password=secret") }), http.MethodGet, "/readyz", nil)
	if notReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("not ready status = %d, want 503", notReady.Code)
	}
	body := decodeBody[ErrorBody](t, notReady)
	if body.Error.Code != "not_ready" {
		t.Errorf("error code = %q", body.Error.Code)
	}
	if bytes.Contains(notReady.Body.Bytes(), []byte("secret")) {
		t.Errorf("probe error leaked to client: %s", notReady.Body.String())
	}
	if body.Error.RequestID == "" {
		t.Errorf("error body has no request id")
	}
}

func TestUnknownRouteReturnsJSONError(t *testing.T) {
	recorder := serve(newTestRouter(func(context.Context) error { return nil }), http.MethodGet, "/v1/nope", nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if body := decodeBody[ErrorBody](t, recorder); body.Error.Code != "not_found" {
		t.Errorf("error code = %q", body.Error.Code)
	}
}

func TestCORSAllowsOnlyConfiguredOrigins(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return nil })

	allowed := serve(router, http.MethodGet, "/healthz", map[string]string{"Origin": "http://localhost:3000"})
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("allowed origin header = %q", got)
	}

	denied := serve(router, http.MethodGet, "/healthz", map[string]string{"Origin": "https://evil.example"})
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unexpected allow-origin for foreign origin: %q", got)
	}
}

func TestPanicsBecomeInternalErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := recoverPanics(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	recorder := serve(handler, http.MethodGet, "/", nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	if body := decodeBody[ErrorBody](t, recorder); body.Error.Code != "internal_error" {
		t.Errorf("error code = %q", body.Error.Code)
	}
}

func TestSecurityHeadersAreSet(t *testing.T) {
	recorder := serve(newTestRouter(func(context.Context) error { return nil }), http.MethodGet, "/healthz", nil)

	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing nosniff header")
	}
	if recorder.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("missing frame options header")
	}
}

func TestCrossSiteWritesAreRejected(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return nil })

	recorder := serve(router, http.MethodPost, "/v1/auth/logout", map[string]string{
		"Origin":         "https://evil.example",
		"Sec-Fetch-Site": "cross-site",
	})

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if body := decodeBody[ErrorBody](t, recorder); body.Error.Code != "cross_origin_rejected" {
		t.Errorf("error code = %q", body.Error.Code)
	}
}

func TestPrivateRoutesRequireSession(t *testing.T) {
	router := newTestRouter(func(context.Context) error { return nil })

	for _, path := range []string{"/v1/me", "/v1/jar", "/v1/payment-links", "/v1/transactions", "/v1/wallet/cleva"} {
		recorder := serve(router, http.MethodGet, path, nil)
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s status = %d, want 401", path, recorder.Code)
		}
	}
}

func TestAttemptLimiterSlidesWindow(t *testing.T) {
	limiter := newAttemptLimiter(2, time.Minute)
	clock := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return clock }

	if !limiter.allow("A@x.com") || !limiter.allow("a@x.com ") {
		t.Fatal("first two attempts must pass")
	}
	if limiter.allow("a@x.com") {
		t.Fatal("third attempt inside the window must be blocked")
	}
	if !limiter.allow("b@x.com") {
		t.Fatal("other keys must not be affected")
	}
	clock = clock.Add(61 * time.Second)
	if !limiter.allow("a@x.com") {
		t.Fatal("attempts must be allowed after the window passes")
	}
}
