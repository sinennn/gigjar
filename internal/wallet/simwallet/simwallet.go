package simwallet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gigjar/internal/database"
	"gigjar/internal/tron"
	"gigjar/internal/wallet"
)

const signatureLifetime = time.Minute

type Wallet struct {
	db                database.Querier
	confirmationDelay time.Duration
}

func New(db database.Querier, confirmationDelay time.Duration) *Wallet {
	return &Wallet{db: db, confirmationDelay: confirmationDelay}
}

func (w *Wallet) Network() tron.Network {
	return tron.NetworkSimulated
}

func (w *Wallet) JarAddress(ctx context.Context, index int64) (tron.Address, error) {
	return tron.SimulatedJarAddress(index), nil
}

func (w *Wallet) PrepareSweep(ctx context.Context, preparation *wallet.Preparation) (wallet.Readiness, error) {
	return wallet.Readiness{Ready: true, Note: "simulated network needs no energy"}, nil
}

type simulatedTx struct {
	Simulated bool   `json:"simulated"`
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    int64  `json:"amount"`
}

func (w *Wallet) SignSweep(ctx context.Context, request wallet.SweepRequest) (wallet.SignedTx, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return wallet.SignedTx{}, err
	}
	raw, err := json.Marshal(simulatedTx{Simulated: true, From: string(request.Jar), To: string(request.Destination), Amount: int64(request.NetAmount)})
	if err != nil {
		return wallet.SignedTx{}, err
	}
	return wallet.SignedTx{TxID: hex.EncodeToString(random), Raw: raw, ExpiresAt: time.Now().Add(signatureLifetime)}, nil
}

func (w *Wallet) Broadcast(ctx context.Context, tx wallet.SignedTx) error {
	var payload simulatedTx
	if err := json.Unmarshal(tx.Raw, &payload); err != nil || !payload.Simulated {
		return fmt.Errorf("%w: not a simulated transaction", wallet.ErrPermanent)
	}
	_, err := w.db.Exec(ctx, `
		INSERT INTO simulated_chain_transfers (tx_hash, from_address, to_address, amount, solidifies_at)
		VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))
		ON CONFLICT (tx_hash) DO NOTHING`,
		tx.TxID, payload.From, payload.To, payload.Amount, w.confirmationDelay.Seconds())
	if err != nil {
		return fmt.Errorf("broadcast simulated sweep: %w", err)
	}
	return nil
}

func (w *Wallet) TransactionKnown(ctx context.Context, txID string) (bool, error) {
	var known bool
	err := w.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM simulated_chain_transfers WHERE tx_hash = $1)`, txID).Scan(&known)
	return known, err
}
