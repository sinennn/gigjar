package httpapi

import (
	"errors"
	"net/http"
	"time"

	"gigjar/internal/chain/simulated"
	"gigjar/internal/paylinks"
	"gigjar/internal/transfers"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

type incomingTransfersPage struct {
	Items      []incomingTransferResponse `json:"items"`
	NextCursor *string                    `json:"next_cursor"`
}

func (a *api) listIncomingTransfers(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(r)
	if !ok {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"limit": "must be between 1 and 100"})
		return
	}
	page, err := transfers.List(r.Context(), a.db, currentUser(r).ID, r.URL.Query().Get("cursor"), limit)
	if errors.Is(err, transfers.ErrInvalidCursor) {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"cursor": "is invalid"})
		return
	}
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	response := incomingTransfersPage{Items: []incomingTransferResponse{}}
	for _, transfer := range page.Transfers {
		response.Items = append(response.Items, newIncomingTransferResponse(transfer))
	}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}
	writeJSON(w, a.logger, http.StatusOK, response)
}

type simulatedTransferRequest struct {
	Reference                string `json:"reference"`
	ToAddress                string `json:"to_address"`
	FromAddress              string `json:"from_address"`
	Amount                   string `json:"amount"`
	Outcome                  string `json:"outcome"`
	ConfirmationDelaySeconds *int   `json:"confirmation_delay_seconds"`
}

type simulatedTransferResponse struct {
	Simulated    bool         `json:"simulated"`
	TxHash       string       `json:"tx_hash"`
	FromAddress  tron.Address `json:"from_address"`
	ToAddress    tron.Address `json:"to_address"`
	Amount       usdt.Amount  `json:"amount"`
	Outcome      string       `json:"outcome"`
	BlockNumber  int64        `json:"block_number"`
	SolidifiesAt time.Time    `json:"solidifies_at"`
}

func (a *api) sendSimulatedTransfer(w http.ResponseWriter, r *http.Request) {
	var request simulatedTransferRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	fields := map[string]string{}
	amount, err := usdt.Parse(request.Amount)
	if err != nil || !amount.IsPositive() {
		fields["amount"] = "must be a positive decimal string with at most 6 fractional digits"
	}
	delay := 10 * time.Second
	if request.ConfirmationDelaySeconds != nil {
		delay = time.Duration(*request.ConfirmationDelaySeconds) * time.Second
	}

	var to tron.Address
	switch {
	case request.Reference != "" && request.ToAddress != "":
		fields["to_address"] = "give either reference or to_address, not both"
	case request.Reference != "":
		instructions, err := a.paylinks.PublicInstructions(r.Context(), request.Reference)
		if errors.Is(err, paylinks.ErrNotFound) {
			fields["reference"] = "no payment link has this reference"
		} else if err != nil {
			a.respondError(w, r, err)
			return
		} else if instructions.PayToAddress == nil {
			fields["reference"] = "the jar for this link has no address yet"
		} else {
			to = *instructions.PayToAddress
		}
	case request.ToAddress != "":
		parsed, err := tron.ParseAddress(request.ToAddress)
		if err != nil {
			fields["to_address"] = "must be a valid TRON address"
		}
		to = parsed
	default:
		fields["reference"] = "give a payment link reference or a to_address"
	}

	var from tron.Address
	if request.FromAddress != "" {
		parsed, err := tron.ParseAddress(request.FromAddress)
		if err != nil {
			fields["from_address"] = "must be a valid TRON address"
		}
		from = parsed
	}
	if len(fields) > 0 {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid", fields)
		return
	}

	sent, err := simulated.Send(r.Context(), a.db, simulated.Transfer{
		To: to, From: from, Amount: amount, Outcome: request.Outcome, ConfirmationDelay: delay,
	})
	if errors.Is(err, simulated.ErrInvalidTransfer) {
		writeError(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", err.Error())
		return
	}
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	a.logger.Info("simulated transfer created", "tx_hash", sent.TxHash, "to", sent.To, "amount", sent.Amount.String(), "outcome", sent.Outcome)
	writeJSON(w, a.logger, http.StatusCreated, simulatedTransferResponse{
		Simulated:    true,
		TxHash:       sent.TxHash,
		FromAddress:  sent.From,
		ToAddress:    sent.To,
		Amount:       sent.Amount,
		Outcome:      sent.Outcome,
		BlockNumber:  sent.BlockNumber,
		SolidifiesAt: sent.SolidifiesAt,
	})
}
