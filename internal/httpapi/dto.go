package httpapi

import (
	"strconv"
	"time"

	"github.com/google/uuid"

	"gigjar/internal/identity"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/paylinks"
	"gigjar/internal/transfers"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

type userResponse struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

func newUserResponse(user identity.User) userResponse {
	return userResponse{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, CreatedAt: user.CreatedAt}
}

type jarBalancesResponse struct {
	Pending   usdt.Amount `json:"pending"`
	Available usdt.Amount `json:"available"`
	Settling  usdt.Amount `json:"settling"`
	Settled   usdt.Amount `json:"settled"`
}

type jarResponse struct {
	Address                  *tron.Address       `json:"address"`
	Network                  tron.Network        `json:"network"`
	Balances                 jarBalancesResponse `json:"balances"`
	SettlementThreshold      usdt.Amount         `json:"settlement_threshold"`
	RemainingUntilSettlement usdt.Amount         `json:"remaining_until_settlement"`
	ClevaDestinationSet      bool                `json:"cleva_destination_set"`
}

func newJarResponse(summary jars.Summary) jarResponse {
	return jarResponse{
		Address: summary.Jar.Address,
		Network: summary.Jar.Network,
		Balances: jarBalancesResponse{
			Pending:   summary.Pending,
			Available: summary.Balances.Available,
			Settling:  summary.Balances.Reserved,
			Settled:   summary.Balances.Settled,
		},
		SettlementThreshold:      summary.SettlementThreshold,
		RemainingUntilSettlement: summary.RemainingUntilSettlement,
		ClevaDestinationSet:      summary.ClevaDestinationSet,
	}
}

type destinationResponse struct {
	Address   tron.Address `json:"address"`
	Network   tron.Network `json:"network"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type paymentLinkResponse struct {
	ID             uuid.UUID       `json:"id"`
	Reference      string          `json:"reference"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	Amount         usdt.Amount     `json:"amount"`
	AmountReceived usdt.Amount     `json:"amount_received"`
	Status         paylinks.Status `json:"status"`
	PayURL         string          `json:"pay_url"`
	CreatedAt      time.Time       `json:"created_at"`
	PaidAt         *time.Time      `json:"paid_at"`
}

type paymentLinkDetailResponse struct {
	paymentLinkResponse
	Transfers []incomingTransferResponse `json:"transfers"`
}

type incomingTransferResponse struct {
	ID            uuid.UUID        `json:"id"`
	Network       tron.Network     `json:"network"`
	TxHash        string           `json:"tx_hash"`
	FromAddress   tron.Address     `json:"from_address"`
	Amount        usdt.Amount      `json:"amount"`
	Status        transfers.Status `json:"status"`
	FailureReason *string          `json:"failure_reason"`
	PaymentLinkID *uuid.UUID       `json:"payment_link_id"`
	BlockNumber   *int64           `json:"block_number"`
	ObservedAt    time.Time        `json:"observed_at"`
	ConfirmedAt   *time.Time       `json:"confirmed_at"`
}

func newIncomingTransferResponse(transfer transfers.Transfer) incomingTransferResponse {
	return incomingTransferResponse{
		ID:            transfer.ID,
		Network:       transfer.Network,
		TxHash:        transfer.TxHash,
		FromAddress:   transfer.From,
		Amount:        transfer.Amount,
		Status:        transfer.Status,
		FailureReason: transfer.FailureReason,
		PaymentLinkID: transfer.PaymentLinkID,
		BlockNumber:   transfer.BlockNumber,
		ObservedAt:    transfer.ObservedAt,
		ConfirmedAt:   transfer.ConfirmedAt,
	}
}

func (a *api) newPaymentLinkResponse(link paylinks.Link) paymentLinkResponse {
	return paymentLinkResponse{
		ID:             link.ID,
		Reference:      link.Reference,
		Title:          link.Title,
		Description:    link.Description,
		Amount:         link.Amount,
		AmountReceived: link.AmountReceived,
		Status:         link.Status,
		PayURL:         a.publicWebURL + "/pay/" + link.Reference,
		CreatedAt:      link.CreatedAt,
		PaidAt:         link.PaidAt,
	}
}

type movementResponse struct {
	ID                 uuid.UUID        `json:"id"`
	Kind               ledger.EntryKind `json:"kind"`
	AvailableDelta     usdt.Amount      `json:"available_delta"`
	SettlingDelta      usdt.Amount      `json:"settling_delta"`
	SettledDelta       usdt.Amount      `json:"settled_delta"`
	IncomingTransferID *uuid.UUID       `json:"incoming_transfer_id"`
	SettlementID       *uuid.UUID       `json:"settlement_id"`
	CreatedAt          time.Time        `json:"created_at"`
}

func newMovementResponse(movement ledger.Movement) movementResponse {
	return movementResponse{
		ID:                 movement.EntryID,
		Kind:               movement.Kind,
		AvailableDelta:     movement.AvailableDelta,
		SettlingDelta:      movement.ReservedDelta,
		SettledDelta:       movement.SettledDelta,
		IncomingTransferID: movement.IncomingTransferID,
		SettlementID:       movement.SettlementID,
		CreatedAt:          movement.CreatedAt,
	}
}

func movementCursor(movement ledger.Movement) string {
	return strconv.FormatInt(movement.Sequence, 10)
}

type tokenResponse struct {
	Symbol          string        `json:"symbol"`
	Standard        string        `json:"standard"`
	ContractAddress *tron.Address `json:"contract_address"`
}

type publicPaymentResponse struct {
	Reference              string          `json:"reference"`
	Title                  string          `json:"title"`
	Description            string          `json:"description"`
	Amount                 usdt.Amount     `json:"amount"`
	RecipientDisplayName   string          `json:"recipient_display_name"`
	PayToAddress           tron.Address    `json:"pay_to_address"`
	Network                tron.Network    `json:"network"`
	Token                  tokenResponse   `json:"token"`
	Status                 paylinks.Status `json:"status"`
	PendingPaymentObserved bool            `json:"pending_payment_observed"`
}

func newPublicPaymentResponse(instructions paylinks.PublicInstructions) publicPaymentResponse {
	token := tokenResponse{Symbol: "USDT", Standard: "TRC20"}
	if contract, ok := instructions.Network.USDTContract(); ok {
		token.ContractAddress = &contract
	}
	return publicPaymentResponse{
		Reference:              instructions.Link.Reference,
		Title:                  instructions.Link.Title,
		Description:            instructions.Link.Description,
		Amount:                 instructions.Link.Amount,
		RecipientDisplayName:   instructions.RecipientDisplayName,
		PayToAddress:           *instructions.PayToAddress,
		Network:                instructions.Network,
		Token:                  token,
		Status:                 instructions.Link.Status,
		PendingPaymentObserved: instructions.PendingObserved,
	}
}
