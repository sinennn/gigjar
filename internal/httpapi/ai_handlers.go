package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"gigjar/internal/ai"
	"gigjar/internal/usdt"
)

type linkDraftRequest struct {
	Brief string `json:"brief"`
}

type linkDraftResponse struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	SuggestedAmount *usdt.Amount `json:"suggested_amount"`
	PricingNote     string       `json:"pricing_note"`
}

type assistantRequest struct {
	Question string `json:"question"`
}

func (a *api) aiAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !a.ai.Enabled() {
		writeError(w, r, a.logger, http.StatusServiceUnavailable, "ai_unavailable", "AI features are not configured")
		return false
	}
	if !a.aiPerUser.allow(currentUser(r).ID.String()) {
		w.Header().Set("Retry-After", "600")
		writeError(w, r, a.logger, http.StatusTooManyRequests, "ai_rate_limited", "you have used the AI features a lot in the last hour; try again later")
		return false
	}
	if !a.aiGlobal.allow("global") {
		w.Header().Set("Retry-After", "30")
		writeError(w, r, a.logger, http.StatusServiceUnavailable, "ai_busy", "the AI is busy right now; try again in a moment")
		return false
	}
	return true
}

func (a *api) respondAIError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *ai.ValidationError
	var busy *ai.BusyError
	switch {
	case errors.As(err, &validation):
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{validation.Field: validation.Message})
	case errors.As(err, &busy):
		seconds := max(int(busy.RetryAfter/time.Second), 10)
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		writeError(w, r, a.logger, http.StatusServiceUnavailable, "ai_busy", "the AI is busy right now; try again in a moment")
	case errors.Is(err, ai.ErrNotConfigured):
		writeError(w, r, a.logger, http.StatusServiceUnavailable, "ai_unavailable", "AI features are not configured")
	case errors.Is(err, ai.ErrBadOutput):
		a.logger.Warn("ai returned unusable output", "request_id", requestID(r), "error", err)
		writeError(w, r, a.logger, http.StatusBadGateway, "ai_failed", "the AI could not produce an answer; try rephrasing")
	default:
		a.respondError(w, r, err)
	}
}

func (a *api) draftPaymentLink(w http.ResponseWriter, r *http.Request) {
	var request linkDraftRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	if !a.aiAllowed(w, r) {
		return
	}
	draft, err := a.ai.DraftLink(r.Context(), request.Brief)
	if err != nil {
		a.respondAIError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, linkDraftResponse(draft))
}

func (a *api) getInsights(w http.ResponseWriter, r *http.Request) {
	if !a.aiAllowed(w, r) {
		return
	}
	insights, err := a.ai.Insights(r.Context(), currentUser(r).ID)
	if err != nil {
		a.respondAIError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, insights)
}

func (a *api) askAssistant(w http.ResponseWriter, r *http.Request) {
	var request assistantRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	if !a.aiAllowed(w, r) {
		return
	}
	answer, err := a.ai.Ask(r.Context(), currentUser(r).ID, request.Question)
	if err != nil {
		a.respondAIError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, answer)
}
