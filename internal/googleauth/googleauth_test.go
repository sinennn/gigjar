package googleauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"gigjar/internal/googleauth/googletest"
)

func newClient(t *testing.T, f *googletest.Provider) *Client {
	t.Helper()
	client, err := New(Config{ClientID: "client-123", ClientSecret: "secret", RedirectURL: "https://gigjar.pages.dev/api/v1/auth/google/callback", Issuer: f.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestHappyPathReturnsVerifiedIdentity(t *testing.T) {
	f := googletest.New(t)
	client := newClient(t, f)
	authURL, flow, err := client.Begin(context.Background(), "/links/new")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(authURL, f.URL+"/auth?") || !strings.Contains(authURL, url.QueryEscape("https://gigjar.pages.dev/api/v1/auth/google/callback")) {
		t.Fatalf("auth url = %s", authURL)
	}
	state, code := f.Authorize(authURL)

	id, returnTo, err := client.Complete(context.Background(), flow, state, code)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "google-sub-1" || id.Email != "Ada@Example.com" || !id.EmailVerified || id.Name != "Ada Lovelace" || returnTo != "/links/new" {
		t.Fatalf("identity = %+v return = %s", id, returnTo)
	}
}

func TestForgedStateIsRejected(t *testing.T) {
	f := googletest.New(t)
	client := newClient(t, f)
	authURL, flow, _ := client.Begin(context.Background(), "/")
	_, code := f.Authorize(authURL)
	if _, _, err := client.Complete(context.Background(), flow, "attacker-state", code); !errors.Is(err, ErrStateMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestTamperedOrExpiredFlowCookieIsRejected(t *testing.T) {
	f := googletest.New(t)
	client := newClient(t, f)
	authURL, flow, _ := client.Begin(context.Background(), "/")
	state, code := f.Authorize(authURL)

	body, signature, _ := strings.Cut(flow, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(body)
	tampered := strings.Replace(string(payload), `"r":"/"`, `"r":"/settings"`, 1)
	if tampered == string(payload) {
		t.Fatal("test did not tamper with the flow")
	}
	forged := base64.RawURLEncoding.EncodeToString([]byte(tampered)) + "." + signature
	if _, _, err := client.Complete(context.Background(), forged, state, code); !errors.Is(err, ErrInvalidFlow) {
		t.Fatalf("tampered flow err = %v", err)
	}

	client.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if _, _, err := client.Complete(context.Background(), flow, state, code); !errors.Is(err, ErrInvalidFlow) {
		t.Fatalf("expired flow err = %v", err)
	}
}

func TestWrongNonceAudienceOrVerifierIsRejected(t *testing.T) {
	cases := map[string]func(f *googletest.Provider){
		"nonce":    func(f *googletest.Provider) { f.NonceOverride = "replayed-nonce" },
		"audience": func(f *googletest.Provider) { f.Audience = "someone-elses-app" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := googletest.New(t)
			mutate(f)
			client := newClient(t, f)
			authURL, flow, _ := client.Begin(context.Background(), "/")
			state, code := f.Authorize(authURL)
			if _, _, err := client.Complete(context.Background(), flow, state, code); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}

	t.Run("pkce", func(t *testing.T) {
		f := googletest.New(t)
		client := newClient(t, f)
		authURL, _, _ := client.Begin(context.Background(), "/")
		state, code := f.Authorize(authURL)
		_, otherFlow, _ := client.Begin(context.Background(), "/")
		body, _, _ := strings.Cut(otherFlow, ".")
		payload, _ := base64.RawURLEncoding.DecodeString(body)
		var flow Flow
		json.Unmarshal(payload, &flow)
		flow.State = state
		reencoded, _ := client.encodeFlow(flow)
		if _, _, err := client.Complete(context.Background(), reencoded, state, code); err == nil {
			t.Fatal("a code redeemed with the wrong PKCE verifier must fail")
		}
	})
}

func TestUnverifiedEmailIsReportedAsUnverified(t *testing.T) {
	f := googletest.New(t)
	f.Verified = false
	client := newClient(t, f)
	authURL, flow, _ := client.Begin(context.Background(), "/")
	state, code := f.Authorize(authURL)
	id, _, err := client.Complete(context.Background(), flow, state, code)
	if err != nil || id.EmailVerified {
		t.Fatalf("identity = %+v err = %v", id, err)
	}
}

func TestSafeReturnPath(t *testing.T) {
	for input, want := range map[string]string{
		"/links/new":          "/links/new",
		"https://evil.com":    "/dashboard",
		"//evil.com":          "/dashboard",
		"/\\evil.com":         "/dashboard",
		"":                    "/dashboard",
		"/ok?x=1":             "/ok?x=1",
		"/a\r\nSet-Cookie: x": "/dashboard",
	} {
		if got := SafeReturnPath(input); got != want {
			t.Errorf("SafeReturnPath(%q) = %q, want %q", input, got, want)
		}
	}
}
