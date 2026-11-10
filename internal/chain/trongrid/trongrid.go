package trongrid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gigjar/internal/chain"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

const (
	transferTopic   = "ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	apiKeyHeader    = "TRON-PRO-API-KEY"
	defaultPageSize = 200
	defaultMaxPages = 5
	maxResponseSize = 4 << 20
)

type Config struct {
	BaseURL    string
	APIKey     string
	Network    tron.Network
	HTTPClient *http.Client
	PageSize   int
	MaxPages   int
}

type Client struct {
	baseURL     string
	apiKey      string
	network     tron.Network
	contract    tron.Address
	contractHex string
	http        *http.Client
	pageSize    int
	maxPages    int
}

func DefaultBaseURL(network tron.Network) (string, bool) {
	switch network {
	case tron.NetworkMainnet:
		return "https://api.trongrid.io", true
	case tron.NetworkNile:
		return "https://nile.trongrid.io", true
	default:
		return "", false
	}
}

func New(cfg Config) (*Client, error) {
	contract, ok := cfg.Network.USDTContract()
	if !ok {
		return nil, fmt.Errorf("network %q has no USDT contract; use the simulated provider", cfg.Network)
	}
	contractHex, err := contract.Hex20()
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid TronGrid base URL %q", cfg.BaseURL)
	}
	client := &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		network:     cfg.Network,
		contract:    contract,
		contractHex: contractHex,
		http:        cfg.HTTPClient,
		pageSize:    cfg.PageSize,
		maxPages:    cfg.MaxPages,
	}
	if client.http == nil {
		client.http = &http.Client{Timeout: 15 * time.Second}
	}
	if client.pageSize <= 0 || client.pageSize > defaultPageSize {
		client.pageSize = defaultPageSize
	}
	if client.maxPages <= 0 {
		client.maxPages = defaultMaxPages
	}
	return client, nil
}

func (c *Client) Network() tron.Network {
	return c.network
}

type trc20Page struct {
	Success bool `json:"success"`
	Data    []struct {
		TransactionID string `json:"transaction_id"`
		TokenInfo     struct {
			Address string `json:"address"`
		} `json:"token_info"`
		BlockTimestamp int64  `json:"block_timestamp"`
		From           string `json:"from"`
		To             string `json:"to"`
		Type           string `json:"type"`
		Value          string `json:"value"`
	} `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

func (c *Client) ListIncoming(ctx context.Context, to tron.Address, since time.Time) (chain.Listing, error) {
	if _, err := tron.ParseAddress(string(to)); err != nil {
		return chain.Listing{}, err
	}
	var (
		listing     chain.Listing
		byHash      = map[string]int{}
		fingerprint string
	)
	for page := 0; page < c.maxPages; page++ {
		query := url.Values{}
		query.Set("only_to", "true")
		query.Set("contract_address", string(c.contract))
		query.Set("limit", strconv.Itoa(c.pageSize))
		query.Set("order_by", "block_timestamp,asc")
		query.Set("min_timestamp", strconv.FormatInt(max(since.UnixMilli(), 0), 10))
		if fingerprint != "" {
			query.Set("fingerprint", fingerprint)
		}
		var response trc20Page
		endpoint := c.baseURL + "/v1/accounts/" + url.PathEscape(string(to)) + "/transactions/trc20?" + query.Encode()
		if err := c.do(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
			return chain.Listing{}, err
		}
		if !response.Success {
			return chain.Listing{}, fmt.Errorf("%w: trc20 listing reported success=false", chain.ErrMalformed)
		}
		for _, item := range response.Data {
			if item.TokenInfo.Address != string(c.contract) || item.Type != "Transfer" || item.To != string(to) {
				continue
			}
			observation, err := parseObservation(item.TransactionID, item.From, item.To, item.Value, item.BlockTimestamp)
			if err != nil {
				return chain.Listing{}, err
			}
			if !observation.Amount.IsPositive() {
				continue
			}
			if observation.BlockTime.After(listing.NewestTimestamp) {
				listing.NewestTimestamp = observation.BlockTime
			}
			if index, seen := byHash[observation.TxHash]; seen {
				total, err := usdt.Add(listing.Observations[index].Amount, observation.Amount)
				if err != nil {
					return chain.Listing{}, fmt.Errorf("%w: amount overflow in %s", chain.ErrMalformed, observation.TxHash)
				}
				listing.Observations[index].Amount = total
				continue
			}
			byHash[observation.TxHash] = len(listing.Observations)
			listing.Observations = append(listing.Observations, observation)
		}
		fingerprint = response.Meta.Fingerprint
		if fingerprint == "" || len(response.Data) == 0 {
			listing.Complete = true
			return listing, nil
		}
	}
	return listing, nil
}

func parseObservation(txID, from, to, value string, blockTimestamp int64) (chain.Observation, error) {
	hash, err := chain.NormalizeTxHash(txID)
	if err != nil {
		return chain.Observation{}, fmt.Errorf("%w: %v", chain.ErrMalformed, err)
	}
	fromAddress, err := tron.ParseAddress(from)
	if err != nil {
		return chain.Observation{}, fmt.Errorf("%w: sender of %s: %v", chain.ErrMalformed, hash, err)
	}
	toAddress, err := tron.ParseAddress(to)
	if err != nil {
		return chain.Observation{}, fmt.Errorf("%w: recipient of %s: %v", chain.ErrMalformed, hash, err)
	}
	amount, err := parseUint(value, 10)
	if err != nil {
		return chain.Observation{}, fmt.Errorf("%w: value of %s: %v", chain.ErrMalformed, hash, err)
	}
	return chain.Observation{
		TxHash:    hash,
		From:      fromAddress,
		To:        toAddress,
		Amount:    amount,
		BlockTime: time.UnixMilli(blockTimestamp).UTC(),
	}, nil
}

type transactionInfo struct {
	ID             string `json:"id"`
	BlockNumber    int64  `json:"blockNumber"`
	BlockTimeStamp int64  `json:"blockTimeStamp"`
	Result         string `json:"result"`
	Receipt        struct {
		Result string `json:"result"`
	} `json:"receipt"`
	Log []struct {
		Address string   `json:"address"`
		Topics  []string `json:"topics"`
		Data    string   `json:"data"`
	} `json:"log"`
}

func (c *Client) TransactionOutcome(ctx context.Context, txHash string) (chain.Outcome, error) {
	hash, err := chain.NormalizeTxHash(txHash)
	if err != nil {
		return chain.Outcome{}, err
	}
	body, err := json.Marshal(map[string]string{"value": hash})
	if err != nil {
		return chain.Outcome{}, err
	}
	var info transactionInfo
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/walletsolidity/gettransactioninfobyid", body, &info); err != nil {
		return chain.Outcome{}, err
	}
	return c.interpret(hash, info)
}

func (c *Client) interpret(hash string, info transactionInfo) (chain.Outcome, error) {
	if info.ID == "" {
		return chain.Outcome{State: chain.OutcomeNotFinal}, nil
	}
	if !strings.EqualFold(info.ID, hash) || info.BlockNumber <= 0 || info.BlockTimeStamp <= 0 {
		return chain.Outcome{}, fmt.Errorf("%w: transaction info for %s does not describe it", chain.ErrMalformed, hash)
	}
	outcome := chain.Outcome{
		BlockNumber: info.BlockNumber,
		BlockTime:   time.UnixMilli(info.BlockTimeStamp).UTC(),
	}
	if info.Result == "FAILED" || info.Receipt.Result != "SUCCESS" {
		reason := strings.ToLower(info.Receipt.Result)
		if reason == "" {
			reason = "failed"
		}
		outcome.State = chain.OutcomeFailed
		outcome.FailureReason = "receipt_" + reason
		return outcome, nil
	}
	outcome.State = chain.OutcomeFinal
	for index, log := range info.Log {
		if !strings.EqualFold(log.Address, c.contractHex) || len(log.Topics) != 3 || !strings.EqualFold(log.Topics[0], transferTopic) {
			continue
		}
		from, err := tron.AddressFromHex(log.Topics[1])
		if err != nil {
			return chain.Outcome{}, fmt.Errorf("%w: log %d sender in %s", chain.ErrMalformed, index, hash)
		}
		to, err := tron.AddressFromHex(log.Topics[2])
		if err != nil {
			return chain.Outcome{}, fmt.Errorf("%w: log %d recipient in %s", chain.ErrMalformed, index, hash)
		}
		amount, err := parseUint(log.Data, 16)
		if err != nil {
			return chain.Outcome{}, fmt.Errorf("%w: log %d value in %s: %v", chain.ErrMalformed, index, hash, err)
		}
		if !amount.IsPositive() {
			continue
		}
		outcome.Transfers = append(outcome.Transfers, chain.TokenTransfer{LogIndex: index, From: from, To: to, Amount: amount})
	}
	return outcome, nil
}

type accountResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		TRC20 []map[string]string `json:"trc20"`
	} `json:"data"`
}

func (c *Client) TokenBalance(ctx context.Context, address tron.Address) (usdt.Amount, error) {
	if _, err := tron.ParseAddress(string(address)); err != nil {
		return 0, err
	}
	var response accountResponse
	endpoint := c.baseURL + "/v1/accounts/" + url.PathEscape(string(address)) + "?only_confirmed=true"
	if err := c.do(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return 0, err
	}
	if !response.Success {
		return 0, fmt.Errorf("%w: account lookup reported success=false", chain.ErrMalformed)
	}
	if len(response.Data) == 0 {
		return 0, nil
	}
	for _, holding := range response.Data[0].TRC20 {
		if value, ok := holding[string(c.contract)]; ok {
			amount, err := parseUint(value, 10)
			if err != nil {
				return 0, fmt.Errorf("%w: balance of %s: %v", chain.ErrMalformed, address, err)
			}
			return amount, nil
		}
	}
	return 0, nil
}

func (c *Client) do(ctx context.Context, method, endpoint string, body []byte, target any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		request.Header.Set(apiKeyHeader, c.apiKey)
	}
	response, err := c.http.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("%w: %s %s: %v", chain.ErrUnavailable, method, request.URL.Path, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("%w: read %s: %v", chain.ErrUnavailable, request.URL.Path, err)
	}
	switch {
	case response.StatusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s", chain.ErrRateLimited, request.URL.Path)
	case response.StatusCode >= 500:
		return fmt.Errorf("%w: %s returned %d", chain.ErrUnavailable, request.URL.Path, response.StatusCode)
	case response.StatusCode != http.StatusOK:
		return fmt.Errorf("trongrid %s returned %d: %s", request.URL.Path, response.StatusCode, truncate(payload, 200))
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("%w: decode %s: %v", chain.ErrMalformed, request.URL.Path, err)
	}
	return nil
}

func parseUint(text string, base int) (usdt.Amount, error) {
	value, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimSpace(text), "0x"), base)
	if !ok || value.Sign() < 0 {
		return 0, fmt.Errorf("not an unsigned integer: %q", truncate([]byte(text), 80))
	}
	if !value.IsInt64() {
		return 0, fmt.Errorf("value %s is out of range", value)
	}
	return usdt.Amount(value.Int64()), nil
}

func truncate(payload []byte, limit int) string {
	if len(payload) <= limit {
		return string(payload)
	}
	return string(payload[:limit]) + "..."
}
