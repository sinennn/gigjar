package tronwallet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gigjar/internal/tron"
	"gigjar/internal/usdt"
	"gigjar/internal/wallet"
)

type SignerClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewSignerClient(baseURL, token string) *SignerClient {
	return &SignerClient{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: &http.Client{Timeout: 30 * time.Second}}
}

type signedResponse struct {
	TxID        string          `json:"tx_id"`
	SignedTx    json.RawMessage `json:"signed_tx"`
	ExpiresAtMs int64           `json:"expires_at_ms"`
	From        string          `json:"from"`
}

func (r signedResponse) toSigned() wallet.SignedTx {
	return wallet.SignedTx{TxID: strings.ToLower(r.TxID), Raw: r.SignedTx, ExpiresAt: time.UnixMilli(r.ExpiresAtMs)}
}

func (c *SignerClient) JarAddress(ctx context.Context, index int64) (tron.Address, error) {
	var response struct {
		Address string `json:"address"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/addresses", map[string]any{"index": index}, &response); err != nil {
		return "", err
	}
	return tron.ParseAddress(response.Address)
}

func (c *SignerClient) TreasuryAddress(ctx context.Context) (tron.Address, error) {
	var response struct {
		Address string `json:"address"`
	}
	if err := c.call(ctx, http.MethodGet, "/v1/treasury", nil, &response); err != nil {
		return "", err
	}
	return tron.ParseAddress(response.Address)
}

func (c *SignerClient) SignUSDTTransfer(ctx context.Context, fromIndex int64, to tron.Address, amount usdt.Amount, feeLimitSun int64) (wallet.SignedTx, tron.Address, error) {
	var response signedResponse
	err := c.call(ctx, http.MethodPost, "/v1/sign/usdt-transfer", map[string]any{
		"from_index":    fromIndex,
		"to":            to,
		"amount":        strconv.FormatInt(int64(amount), 10),
		"fee_limit_sun": feeLimitSun,
	}, &response)
	if err != nil {
		return wallet.SignedTx{}, "", err
	}
	return response.toSigned(), tron.Address(response.From), nil
}

func (c *SignerClient) SignTRXTransfer(ctx context.Context, to tron.Address, amountSun int64, purpose string) (wallet.SignedTx, error) {
	var response signedResponse
	err := c.call(ctx, http.MethodPost, "/v1/sign/trx-transfer", map[string]any{
		"to":         to,
		"amount_sun": amountSun,
		"purpose":    purpose,
	}, &response)
	if err != nil {
		return wallet.SignedTx{}, err
	}
	return response.toSigned(), nil
}

func (c *SignerClient) call(ctx context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("signer %s: %w", path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("signer %s: %w", path, err)
	}
	switch {
	case response.StatusCode == http.StatusUnprocessableEntity || response.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%w: signer refused %s (%d): %s", wallet.ErrPermanent, path, response.StatusCode, truncate(payload))
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("signer %s returned %d: %s", path, response.StatusCode, truncate(payload))
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("signer %s returned invalid JSON: %w", path, err)
	}
	return nil
}

func truncate(payload []byte) string {
	if len(payload) > 200 {
		return string(payload[:200]) + "..."
	}
	return string(payload)
}

var errUnexpectedSigner = errors.New("signer signed from an unexpected address")
