package googleauth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"gigjar/internal/identity"
)

const (
	DefaultIssuer = "https://accounts.google.com"
	flowLifetime  = 10 * time.Minute
)

var (
	ErrInvalidFlow   = errors.New("sign-in flow is missing, expired or was tampered with")
	ErrStateMismatch = errors.New("sign-in state does not match")
	ErrNonceMismatch = errors.New("ID token nonce does not match")
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Issuer       string
	HTTPClient   *http.Client
}

type Client struct {
	cfg      Config
	flowKey  []byte
	mu       sync.Mutex
	provider *oidc.Provider
	now      func() time.Time
}

type Flow struct {
	State     string `json:"s"`
	Nonce     string `json:"n"`
	Verifier  string `json:"v"`
	ReturnTo  string `json:"r"`
	ExpiresAt int64  `json:"e"`
}

func New(cfg Config) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURL == "" {
		return nil, errors.New("google sign-in needs a client id, client secret and redirect url")
	}
	if cfg.Issuer == "" {
		cfg.Issuer = DefaultIssuer
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	key := sha256.Sum256([]byte("gigjar-google-flow/" + cfg.ClientSecret))
	return &Client{cfg: cfg, flowKey: key[:], now: time.Now}, nil
}

func (c *Client) oidcProvider(ctx context.Context) (*oidc.Provider, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return c.provider, nil
	}
	provider, err := oidc.NewProvider(oidc.ClientContext(ctx, c.cfg.HTTPClient), c.cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discover google openid configuration: %w", err)
	}
	c.provider = provider
	return provider, nil
}

func (c *Client) oauthConfig(provider *oidc.Provider) oauth2.Config {
	return oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		RedirectURL:  c.cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

func (c *Client) Begin(ctx context.Context, returnTo string) (string, string, error) {
	provider, err := c.oidcProvider(ctx)
	if err != nil {
		return "", "", err
	}
	flow := Flow{
		State:     randomToken(),
		Nonce:     randomToken(),
		Verifier:  oauth2.GenerateVerifier(),
		ReturnTo:  SafeReturnPath(returnTo),
		ExpiresAt: c.now().Add(flowLifetime).Unix(),
	}
	config := c.oauthConfig(provider)
	authURL := config.AuthCodeURL(flow.State,
		oidc.Nonce(flow.Nonce),
		oauth2.S256ChallengeOption(flow.Verifier),
		oauth2.SetAuthURLParam("prompt", "select_account"))
	encoded, err := c.encodeFlow(flow)
	if err != nil {
		return "", "", err
	}
	return authURL, encoded, nil
}

func (c *Client) Complete(ctx context.Context, encodedFlow, state, code string) (identity.ExternalIdentity, string, error) {
	flow, err := c.decodeFlow(encodedFlow)
	if err != nil {
		return identity.ExternalIdentity{}, "/", err
	}
	if subtle.ConstantTimeCompare([]byte(flow.State), []byte(state)) != 1 {
		return identity.ExternalIdentity{}, flow.ReturnTo, ErrStateMismatch
	}
	if code == "" {
		return identity.ExternalIdentity{}, flow.ReturnTo, errors.New("google returned no authorization code")
	}
	provider, err := c.oidcProvider(ctx)
	if err != nil {
		return identity.ExternalIdentity{}, flow.ReturnTo, err
	}
	httpCtx := oidc.ClientContext(ctx, c.cfg.HTTPClient)
	config := c.oauthConfig(provider)
	token, err := config.Exchange(httpCtx, code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		return identity.ExternalIdentity{}, flow.ReturnTo, fmt.Errorf("exchange authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return identity.ExternalIdentity{}, flow.ReturnTo, errors.New("google returned no id token")
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: c.cfg.ClientID, Now: c.now}).Verify(httpCtx, rawIDToken)
	if err != nil {
		return identity.ExternalIdentity{}, flow.ReturnTo, fmt.Errorf("verify id token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(flow.Nonce)) != 1 {
		return identity.ExternalIdentity{}, flow.ReturnTo, ErrNonceMismatch
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return identity.ExternalIdentity{}, flow.ReturnTo, fmt.Errorf("read id token claims: %w", err)
	}
	return identity.ExternalIdentity{
		Provider:      identity.ProviderGoogle,
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified == true || claims.EmailVerified == "true",
		Name:          claims.Name,
	}, flow.ReturnTo, nil
}

func (c *Client) encodeFlow(flow Flow) (string, error) {
	payload, err := json.Marshal(flow)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + c.sign(body), nil
}

func (c *Client) decodeFlow(encoded string) (Flow, error) {
	body, signature, ok := strings.Cut(encoded, ".")
	if !ok || !hmac.Equal([]byte(signature), []byte(c.sign(body))) {
		return Flow{}, ErrInvalidFlow
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Flow{}, ErrInvalidFlow
	}
	var flow Flow
	if err := json.Unmarshal(payload, &flow); err != nil || flow.State == "" || flow.Nonce == "" || flow.Verifier == "" {
		return Flow{}, ErrInvalidFlow
	}
	if c.now().Unix() > flow.ExpiresAt {
		return Flow{}, ErrInvalidFlow
	}
	flow.ReturnTo = SafeReturnPath(flow.ReturnTo)
	return flow, nil
}

func (c *Client) sign(body string) string {
	mac := hmac.New(sha256.New, c.flowKey)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func SafeReturnPath(path string) string {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\\\r\n") || len(path) > 512 {
		return "/dashboard"
	}
	return path
}

func randomToken() string {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}
