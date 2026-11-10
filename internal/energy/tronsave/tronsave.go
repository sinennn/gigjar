package tronsave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gigjar/internal/energy"
	"gigjar/internal/tron"
)

const MinimumOrder = 65_000

type Config struct {
	BaseURL         string
	PaymentAddress  tron.Address
	DurationSeconds int
	MaxUnitPriceSun float64
	HTTPClient      *http.Client
	Pay             energy.PaymentSigner
}

type Client struct {
	cfg Config
}

func Defaults(network tron.Network) (baseURL string, paymentAddress tron.Address, ok bool) {
	switch network {
	case tron.NetworkMainnet:
		return "https://api.tronsave.io", "TWZEhq5JuUVvGtutNgnRBATbF8BnHGyn4S", true
	case tron.NetworkNile:
		return "https://api-dev.tronsave.io", "TATT1UzHRikft98bRFqApFTsaSw73ycfoS", true
	default:
		return "", "", false
	}
}

func New(cfg Config) (*Client, error) {
	if cfg.Pay == nil {
		return nil, fmt.Errorf("tronsave: a payment signer is required")
	}
	if _, err := tron.ParseAddress(string(cfg.PaymentAddress)); err != nil {
		return nil, fmt.Errorf("tronsave payment address: %w", err)
	}
	if cfg.DurationSeconds < 900 {
		cfg.DurationSeconds = 900
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Client{cfg: cfg}, nil
}

func (c *Client) Name() string {
	return "tronsave"
}

type envelope struct {
	Error   bool            `json:"error"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) Quote(ctx context.Context, receiver tron.Address, amount int64) (energy.Quote, error) {
	var data struct {
		UnitPrice         float64 `json:"unitPrice"`
		EstimateTrx       int64   `json:"estimateTrx"`
		AvailableResource int64   `json:"availableResource"`
	}
	err := c.post(ctx, "/v2/estimate-buy-resource", map[string]any{
		"receiver":       receiver,
		"resourceType":   "ENERGY",
		"resourceAmount": amount,
		"durationSec":    c.cfg.DurationSeconds,
		"unitPrice":      "MEDIUM",
	}, &data)
	if err != nil {
		return energy.Quote{}, err
	}
	return energy.Quote{UnitPriceSun: data.UnitPrice, TotalSun: data.EstimateTrx, Available: data.AvailableResource}, nil
}

func (c *Client) Rent(ctx context.Context, receiver tron.Address, amount int64) (energy.Rental, error) {
	amount = max(amount, MinimumOrder)
	quote, err := c.Quote(ctx, receiver, amount)
	if err != nil {
		return energy.Rental{}, err
	}
	if quote.Available < amount {
		return energy.Rental{}, fmt.Errorf("%w: %d of %d available", energy.ErrUnavailable, quote.Available, amount)
	}
	if c.cfg.MaxUnitPriceSun > 0 && quote.UnitPriceSun > c.cfg.MaxUnitPriceSun {
		return energy.Rental{}, fmt.Errorf("%w: %.2f sun > %.2f sun", energy.ErrTooExpensive, quote.UnitPriceSun, c.cfg.MaxUnitPriceSun)
	}
	if quote.TotalSun <= 0 {
		return energy.Rental{}, fmt.Errorf("tronsave quoted a non-positive total")
	}
	signedTx, paymentTxID, err := c.cfg.Pay(ctx, c.cfg.PaymentAddress, quote.TotalSun)
	if err != nil {
		return energy.Rental{}, fmt.Errorf("sign tronsave payment: %w", err)
	}
	var data struct {
		OrderID string `json:"orderId"`
	}
	err = c.post(ctx, "/v2/buy-resource", map[string]any{
		"resourceType":   "ENERGY",
		"unitPrice":      quote.UnitPriceSun,
		"resourceAmount": amount,
		"receiver":       receiver,
		"durationSec":    c.cfg.DurationSeconds,
		"signedTx":       json.RawMessage(signedTx),
		"options": map[string]any{
			"allowPartialFill":        false,
			"onlyCreateWhenFulfilled": true,
			"maxPriceAccepted":        c.cfg.MaxUnitPriceSun,
		},
	}, &data)
	if err != nil {
		return energy.Rental{}, fmt.Errorf("tronsave order (payment tx %s): %w", paymentTxID, err)
	}
	if data.OrderID == "" {
		return energy.Rental{}, fmt.Errorf("tronsave returned no order id (payment tx %s)", paymentTxID)
	}
	return energy.Rental{Reference: data.OrderID, Amount: amount, TotalSun: quote.TotalSun, UnitPriceSun: quote.UnitPriceSun, Provider: c.Name()}, nil
}

func (c *Client) post(ctx context.Context, path string, body any, target any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.cfg.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("tronsave %s: %w", path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("tronsave %s: %w", path, err)
	}
	var result envelope
	if err := json.Unmarshal(payload, &result); err != nil {
		return fmt.Errorf("tronsave %s returned %d with invalid JSON", path, response.StatusCode)
	}
	if result.Error || response.StatusCode < 200 || response.StatusCode > 299 {
		message := result.Message
		if strings.Contains(message, "UNIT_PRICE_TOO_LOW") || strings.Contains(strings.ToUpper(message), "PRICE") {
			return fmt.Errorf("%w: %s", energy.ErrTooExpensive, message)
		}
		return fmt.Errorf("tronsave %s failed (%d): %s", path, response.StatusCode, message)
	}
	if target != nil {
		if err := json.Unmarshal(result.Data, target); err != nil {
			return fmt.Errorf("tronsave %s returned unexpected data: %w", path, err)
		}
	}
	return nil
}
