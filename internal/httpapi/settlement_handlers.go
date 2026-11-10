package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"gigjar/internal/settlement"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

type settlementResponse struct {
	ID                 uuid.UUID        `json:"id"`
	Network            tron.Network     `json:"network"`
	Amount             usdt.Amount      `json:"amount"`
	Fee                usdt.Amount      `json:"fee"`
	NetAmount          usdt.Amount      `json:"net_amount"`
	DestinationAddress tron.Address     `json:"destination_address"`
	State              settlement.State `json:"state"`
	TxID               *string          `json:"tx_id"`
	FailureReason      *string          `json:"failure_reason"`
	CreatedAt          time.Time        `json:"created_at"`
	ConfirmedAt        *time.Time       `json:"confirmed_at"`
}

type settlementsPage struct {
	Items      []settlementResponse `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

func newSettlementResponse(s settlement.Settlement) settlementResponse {
	response := settlementResponse{
		ID:                 s.ID,
		Network:            s.Network,
		Amount:             s.Amount,
		Fee:                s.Fee,
		NetAmount:          s.Net(),
		DestinationAddress: s.Destination,
		State:              s.State,
		FailureReason:      s.FailureReason,
		CreatedAt:          s.CreatedAt,
		ConfirmedAt:        s.ConfirmedAt,
	}
	if s.State == settlement.StateBroadcast || s.State == settlement.StateConfirmed || s.State == settlement.StateFailed {
		response.TxID = s.TxID
	}
	return response
}

func (a *api) listSettlements(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(r)
	if !ok {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"limit": "must be between 1 and 100"})
		return
	}
	page, err := settlement.List(r.Context(), a.db, currentUser(r).ID, r.URL.Query().Get("cursor"), limit)
	if errors.Is(err, settlement.ErrInvalidCursor) {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"cursor": "is invalid"})
		return
	}
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	response := settlementsPage{Items: []settlementResponse{}}
	for _, item := range page.Settlements {
		response.Items = append(response.Items, newSettlementResponse(item))
	}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}
	writeJSON(w, a.logger, http.StatusOK, response)
}
