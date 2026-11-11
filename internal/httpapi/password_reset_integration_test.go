//go:build integration

package httpapi_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gigjar/internal/httpapi"
	"gigjar/internal/mail"
)

type capturingMailer struct {
	mu       sync.Mutex
	messages []mail.Message
}

func (c *capturingMailer) Configured() bool { return true }

func (c *capturingMailer) Send(ctx context.Context, message mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, message)
	return nil
}

var resetLink = regexp.MustCompile(`http://localhost:3000/reset-password\?token=(\S+)`)

func (c *capturingMailer) waitForToken(t *testing.T, to string, count int) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		var matching []mail.Message
		for _, m := range c.messages {
			if strings.EqualFold(m.To, to) {
				matching = append(matching, m)
			}
		}
		c.mu.Unlock()
		if len(matching) >= count {
			found := resetLink.FindStringSubmatch(matching[count-1].Text)
			if found == nil {
				t.Fatalf("reset email has no link: %q", matching[count-1].Text)
			}
			token, err := url.QueryUnescape(found[1])
			if err != nil {
				t.Fatal(err)
			}
			return token
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no reset email #%d for %s", count, to)
	return ""
}

func (c *capturingMailer) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.messages)
}

func TestPasswordResetFlow(t *testing.T) {
	mailer := &capturingMailer{}
	pool, server := startAPI(t, func(cfg *httpapi.RouterConfig) { cfg.Mailer = mailer })

	owner := newClient(t, server)
	owner.register("reset@example.com")
	otherDevice := newClient(t, server)
	otherDevice.expect(http.StatusOK, http.MethodPost, "/v1/auth/login", map[string]string{"email": "reset@example.com", "password": "long enough password"})

	anon := newClient(t, server)
	unknown := anon.expect(http.StatusAccepted, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "nobody@example.com"})
	known := anon.expect(http.StatusAccepted, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "Reset@Example.com"})
	if unknown["status"] != known["status"] {
		t.Fatal("responses must not reveal whether an account exists")
	}
	firstToken := mailer.waitForToken(t, "reset@example.com", 1)
	anon.expect(http.StatusAccepted, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "reset@example.com"})
	token := mailer.waitForToken(t, "reset@example.com", 2)
	time.Sleep(200 * time.Millisecond)
	if mailer.count() != 2 {
		t.Fatalf("an unknown email must not receive mail; sent %d", mailer.count())
	}

	status, body := anon.do(http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": firstToken, "password": "a brand new password"}, nil)
	if status != http.StatusBadRequest || errorCode(body) != "reset_token_invalid" {
		t.Fatalf("an older link must stop working once a newer one is sent: %d %v", status, body)
	}
	if body := anon.expect(http.StatusUnprocessableEntity, http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": token, "password": "short"}); errorCode(body) != "validation_failed" {
		t.Fatalf("short password = %v", body)
	}

	resetter := newClient(t, server)
	user := resetter.expect(http.StatusOK, http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": token, "password": "a brand new password"})
	if user["email"] != "reset@example.com" {
		t.Fatalf("reset user = %v", user)
	}
	resetter.expect(http.StatusOK, http.MethodGet, "/v1/me", nil)
	owner.expect(http.StatusUnauthorized, http.MethodGet, "/v1/me", nil)
	otherDevice.expect(http.StatusUnauthorized, http.MethodGet, "/v1/me", nil)

	status, body = anon.do(http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": token, "password": "yet another password"}, nil)
	if status != http.StatusBadRequest || errorCode(body) != "reset_token_invalid" {
		t.Fatalf("a used link must not work twice: %d %v", status, body)
	}
	newClient(t, server).expect(http.StatusUnauthorized, http.MethodPost, "/v1/auth/login", map[string]string{"email": "reset@example.com", "password": "long enough password"})
	newClient(t, server).expect(http.StatusOK, http.MethodPost, "/v1/auth/login", map[string]string{"email": "reset@example.com", "password": "a brand new password"})

	anon.expect(http.StatusAccepted, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "reset@example.com"})
	expiring := mailer.waitForToken(t, "reset@example.com", 3)
	if _, err := pool.Exec(context.Background(), `UPDATE password_resets SET created_at = now() - interval '1 hour', expires_at = now() - interval '30 minutes' WHERE used_at IS NULL`); err != nil {
		t.Fatal(err)
	}
	status, body = anon.do(http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": expiring, "password": "a third new password"}, nil)
	if status != http.StatusBadRequest || errorCode(body) != "reset_token_invalid" {
		t.Fatalf("an expired link must not work: %d %v", status, body)
	}
	status, body = anon.do(http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": "made-up-token", "password": "a third new password"}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("an invented token = %d %v", status, body)
	}
	var hashed, plain int
	err := pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE token_hash = sha256(convert_to($1, 'UTF8'))),
		       count(*) FILTER (WHERE token_hash = convert_to($1, 'UTF8'))
		FROM password_resets`, expiring).Scan(&hashed, &plain)
	if err != nil || hashed != 1 || plain != 0 {
		t.Fatalf("tokens must be stored as SHA-256 hashes only: hashed=%d plain=%d err=%v", hashed, plain, err)
	}
}

func TestForgotPasswordIsRateLimitedAndValidated(t *testing.T) {
	mailer := &capturingMailer{}
	_, server := startAPI(t, func(cfg *httpapi.RouterConfig) { cfg.Mailer = mailer })
	anon := newClient(t, server)
	anon.expect(http.StatusUnprocessableEntity, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "not-an-email"})
	for range 5 {
		anon.expect(http.StatusAccepted, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "flood@example.com"})
	}
	if body := anon.expect(http.StatusTooManyRequests, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "FLOOD@example.com"}); errorCode(body) != "too_many_attempts" {
		t.Fatalf("rate limit = %v", body)
	}
}

func TestGoogleOnlyAccountCanSetAPasswordByReset(t *testing.T) {
	mailer := &capturingMailer{}
	pool, server := startAPI(t, func(cfg *httpapi.RouterConfig) { cfg.Mailer = mailer })
	if _, err := pool.Exec(context.Background(), `INSERT INTO users (email, password_hash, display_name) VALUES ('googler@example.com', NULL, 'Googler')`); err != nil {
		t.Fatal(err)
	}
	anon := newClient(t, server)
	anon.expect(http.StatusAccepted, http.MethodPost, "/v1/auth/password/forgot", map[string]string{"email": "googler@example.com"})
	token := mailer.waitForToken(t, "googler@example.com", 1)
	anon.expect(http.StatusOK, http.MethodPost, "/v1/auth/password/reset", map[string]string{"token": token, "password": "now i have a password"})
	newClient(t, server).expect(http.StatusOK, http.MethodPost, "/v1/auth/login", map[string]string{"email": "googler@example.com", "password": "now i have a password"})
}

func TestSwaggerDocsAreServed(t *testing.T) {
	_, server := startAPI(t)
	for path, want := range map[string]string{"/docs": "swagger-ui-bundle.js", "/openapi.yaml": "operationId: forgotPassword"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.Contains(string(raw), want) {
			t.Fatalf("%s = %d, missing %q", path, response.StatusCode, want)
		}
	}
}
