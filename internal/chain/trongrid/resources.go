package trongrid

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"gigjar/internal/chain"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var ErrBroadcastRejected = errors.New("node rejected the transaction")

type AccountResources struct {
	Exists          bool
	BalanceSun      int64
	EnergyAvailable int64
	BandwidthFree   int64
}

func (c *Client) AccountResources(ctx context.Context, address tron.Address) (AccountResources, error) {
	if _, err := tron.ParseAddress(string(address)); err != nil {
		return AccountResources{}, err
	}
	var account struct {
		Address string `json:"address"`
		Balance int64  `json:"balance"`
	}
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/wallet/getaccount", mustJSON(map[string]any{"address": address, "visible": true}), &account); err != nil {
		return AccountResources{}, err
	}
	if account.Address == "" {
		return AccountResources{}, nil
	}
	var resource struct {
		FreeNetLimit int64 `json:"freeNetLimit"`
		FreeNetUsed  int64 `json:"freeNetUsed"`
		NetLimit     int64 `json:"NetLimit"`
		NetUsed      int64 `json:"NetUsed"`
		EnergyLimit  int64 `json:"EnergyLimit"`
		EnergyUsed   int64 `json:"EnergyUsed"`
	}
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/wallet/getaccountresource", mustJSON(map[string]any{"address": address, "visible": true}), &resource); err != nil {
		return AccountResources{}, err
	}
	return AccountResources{
		Exists:          true,
		BalanceSun:      account.Balance,
		EnergyAvailable: max(resource.EnergyLimit-resource.EnergyUsed, 0),
		BandwidthFree:   max(resource.FreeNetLimit-resource.FreeNetUsed, 0) + max(resource.NetLimit-resource.NetUsed, 0),
	}, nil
}

func (c *Client) EstimateUSDTTransferEnergy(ctx context.Context, from, to tron.Address, amount usdt.Amount) (int64, error) {
	parameter, err := encodeTransferCall(to, amount)
	if err != nil {
		return 0, err
	}
	var response struct {
		Result struct {
			Result  bool   `json:"result"`
			Message string `json:"message"`
		} `json:"result"`
		EnergyUsed int64 `json:"energy_used"`
	}
	body := mustJSON(map[string]any{
		"owner_address":     from,
		"contract_address":  c.contract,
		"function_selector": "transfer(address,uint256)",
		"parameter":         parameter,
		"visible":           true,
	})
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/wallet/triggerconstantcontract", body, &response); err != nil {
		return 0, err
	}
	if !response.Result.Result || response.EnergyUsed <= 0 {
		message := response.Result.Message
		if decoded, err := hex.DecodeString(message); err == nil {
			message = string(decoded)
		}
		return 0, fmt.Errorf("%w: energy estimate failed: %s", chain.ErrMalformed, truncate([]byte(message), 200))
	}
	return response.EnergyUsed, nil
}

func (c *Client) EnergyFeeSun(ctx context.Context) (int64, error) {
	var response struct {
		ChainParameter []struct {
			Key   string `json:"key"`
			Value int64  `json:"value"`
		} `json:"chainParameter"`
	}
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/wallet/getchainparameters", mustJSON(map[string]any{}), &response); err != nil {
		return 0, err
	}
	for _, parameter := range response.ChainParameter {
		if parameter.Key == "getEnergyFee" && parameter.Value > 0 {
			return parameter.Value, nil
		}
	}
	return 0, fmt.Errorf("%w: getEnergyFee missing from chain parameters", chain.ErrMalformed)
}

func (c *Client) Broadcast(ctx context.Context, signed json.RawMessage) error {
	var response struct {
		Result  bool   `json:"result"`
		Code    string `json:"code"`
		Message string `json:"message"`
		TxID    string `json:"txid"`
	}
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/wallet/broadcasttransaction", signed, &response); err != nil {
		return err
	}
	if response.Result || response.Code == "DUP_TRANSACTION_ERROR" {
		return nil
	}
	message := response.Message
	if decoded, err := hex.DecodeString(message); err == nil {
		message = string(decoded)
	}
	return fmt.Errorf("%w: %s %s", ErrBroadcastRejected, response.Code, truncate([]byte(message), 200))
}

func (c *Client) TransactionKnown(ctx context.Context, txID string) (bool, error) {
	hash, err := chain.NormalizeTxHash(txID)
	if err != nil {
		return false, err
	}
	var response struct {
		TxID string `json:"txID"`
	}
	if err := c.do(ctx, http.MethodPost, c.baseURL+"/wallet/gettransactionbyid", mustJSON(map[string]string{"value": hash}), &response); err != nil {
		return false, err
	}
	return strings.EqualFold(response.TxID, hash), nil
}

func encodeTransferCall(to tron.Address, amount usdt.Amount) (string, error) {
	recipient, err := to.Hex20()
	if err != nil {
		return "", err
	}
	if !amount.IsPositive() {
		return "", errors.New("transfer amount must be positive")
	}
	value := fmt.Sprintf("%064x", new(big.Int).SetInt64(int64(amount)))
	return strings.Repeat("0", 24) + recipient + value, nil
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
