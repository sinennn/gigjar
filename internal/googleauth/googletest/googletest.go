package googletest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
)

type Provider struct {
	URL           string
	ClientID      string
	Subject       string
	Email         string
	Verified      bool
	Name          string
	Audience      string
	NonceOverride string

	t          testing.TB
	key        *rsa.PrivateKey
	mu         sync.Mutex
	challenges map[string]string
	nonces     map[string]string
}

func New(t testing.TB) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{t: t, key: key, ClientID: "client-123", Subject: "google-sub-1", Email: "Ada@Example.com", Verified: true, Name: "Ada Lovelace",
		challenges: map[string]string{}, nonces: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.URL,
			"authorization_endpoint":                p.URL + "/auth",
			"token_endpoint":                        p.URL + "/token",
			"jwks_uri":                              p.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		code := r.Form.Get("code")
		p.mu.Lock()
		challenge, nonce := p.challenges[code], p.nonces[code]
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		w.Header().Set("Content-Type", "application/json")
		if challenge == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "token_type": "Bearer", "expires_in": 3600, "id_token": p.idToken(nonce)})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	p.URL = server.URL
	return p
}

func (p *Provider) idToken(nonce string) string {
	audience := p.Audience
	if audience == "" {
		audience = p.ClientID
	}
	if p.NonceOverride != "" {
		nonce = p.NonceOverride
	}
	claims := map[string]any{
		"iss": p.URL, "aud": audience, "sub": p.Subject,
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"nonce": nonce, "email": p.Email, "email_verified": p.Verified, "name": p.Name,
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.key}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if err != nil {
		p.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	signed, err := signer.Sign(payload)
	if err != nil {
		p.t.Fatal(err)
	}
	compact, _ := signed.CompactSerialize()
	return compact
}

func (p *Provider) Authorize(authURL string) (state, code string) {
	p.t.Helper()
	parsed, err := url.Parse(authURL)
	if err != nil {
		p.t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" || query.Get("nonce") == "" {
		p.t.Fatalf("auth url lacks PKCE or nonce: %s", authURL)
	}
	if !strings.Contains(query.Get("scope"), "openid") || !strings.Contains(query.Get("scope"), "email") {
		p.t.Fatalf("scope = %q", query.Get("scope"))
	}
	code = "code-" + query.Get("state")[:12]
	p.mu.Lock()
	p.challenges[code] = query.Get("code_challenge")
	p.nonces[code] = query.Get("nonce")
	p.mu.Unlock()
	return query.Get("state"), code
}
