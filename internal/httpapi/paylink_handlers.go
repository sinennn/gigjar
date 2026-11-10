package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"gigjar/internal/paylinks"
	"gigjar/internal/usdt"
)

type createPaymentLinkRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Amount      string `json:"amount"`
}

type paymentLinksPage struct {
	Items      []paymentLinkResponse `json:"items"`
	NextCursor *string               `json:"next_cursor"`
}

func (a *api) createPaymentLink(w http.ResponseWriter, r *http.Request) {
	var request createPaymentLinkRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	amount, err := usdt.Parse(request.Amount)
	if err != nil {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"amount": "must be a decimal string with at most 6 fractional digits, like \"4.50\""})
		return
	}
	link, created, err := a.paylinks.Create(r.Context(), currentUser(r).ID, paylinks.CreateInput{
		Title:          request.Title,
		Description:    request.Description,
		Amount:         amount,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, a.logger, status, a.newPaymentLinkResponse(link))
}

func (a *api) listPaymentLinks(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(r)
	if !ok {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"limit": "must be between 1 and 100"})
		return
	}
	filter := paylinks.ListFilter{Cursor: r.URL.Query().Get("cursor"), Limit: limit}
	if text := r.URL.Query().Get("status"); text != "" {
		status, ok := paylinks.ParseStatus(text)
		if !ok {
			writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
				map[string]string{"status": "must be open, paid or closed"})
			return
		}
		filter.Status = &status
	}
	page, err := a.paylinks.List(r.Context(), currentUser(r).ID, filter)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	response := paymentLinksPage{Items: []paymentLinkResponse{}}
	for _, link := range page.Links {
		response.Items = append(response.Items, a.newPaymentLinkResponse(link))
	}
	if page.NextCursor != "" {
		response.NextCursor = &page.NextCursor
	}
	writeJSON(w, a.logger, http.StatusOK, response)
}

func (a *api) getPaymentLink(w http.ResponseWriter, r *http.Request) {
	linkID, ok := a.linkIDParam(w, r)
	if !ok {
		return
	}
	link, err := a.paylinks.Get(r.Context(), currentUser(r).ID, linkID)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, a.newPaymentLinkResponse(link))
}

func (a *api) closePaymentLink(w http.ResponseWriter, r *http.Request) {
	linkID, ok := a.linkIDParam(w, r)
	if !ok {
		return
	}
	link, err := a.paylinks.Close(r.Context(), currentUser(r).ID, linkID)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, a.newPaymentLinkResponse(link))
}

func (a *api) getPublicPayment(w http.ResponseWriter, r *http.Request) {
	instructions, err := a.paylinks.PublicInstructions(r.Context(), chi.URLParam(r, "reference"))
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	if instructions.PayToAddress == nil {
		writeError(w, r, a.logger, http.StatusServiceUnavailable, "jar_not_ready",
			"this jar is still being set up, so it cannot receive payments yet")
		return
	}
	writeJSON(w, a.logger, http.StatusOK, newPublicPaymentResponse(instructions))
}

func (a *api) linkIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	linkID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		a.respondError(w, r, paylinks.ErrNotFound)
		return uuid.Nil, false
	}
	return linkID, true
}
