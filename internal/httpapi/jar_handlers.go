package httpapi

import (
	"net/http"
	"strconv"

	"gigjar/internal/ledger"
	"gigjar/internal/tron"
)

type setDestinationRequest struct {
	Address string `json:"address"`
}

type movementsPage struct {
	Items      []movementResponse `json:"items"`
	NextCursor *string            `json:"next_cursor"`
}

func (a *api) getJar(w http.ResponseWriter, r *http.Request) {
	summary, err := a.jars.Summary(r.Context(), currentUser(r).ID)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, newJarResponse(summary))
}

func (a *api) getDestination(w http.ResponseWriter, r *http.Request) {
	destination, err := a.jars.CurrentDestination(r.Context(), currentUser(r).ID)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, destinationResponse{
		Address:   destination.Address,
		Network:   a.network,
		UpdatedAt: destination.CreatedAt,
	})
}

func (a *api) setDestination(w http.ResponseWriter, r *http.Request) {
	var request setDestinationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	address, err := tron.ParseAddress(request.Address)
	if err != nil {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"address": "must be a TRON (TRC20) address starting with T"})
		return
	}
	if tron.IsKnownUSDTContract(address) {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"address": "this is the USDT token contract, not your Cleva deposit address"})
		return
	}
	destination, err := a.jars.SetDestination(r.Context(), currentUser(r).ID, address)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	writeJSON(w, a.logger, http.StatusOK, destinationResponse{
		Address:   destination.Address,
		Network:   a.network,
		UpdatedAt: destination.CreatedAt,
	})
}

func (a *api) listTransactions(w http.ResponseWriter, r *http.Request) {
	limit, ok := queryLimit(r)
	if !ok {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"limit": "must be between 1 and 100"})
		return
	}
	var before int64
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		parsed, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || parsed <= 0 {
			writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
				map[string]string{"cursor": "is invalid"})
			return
		}
		before = parsed
	}
	movements, err := ledger.ListMovements(r.Context(), a.db, currentUser(r).ID, before, limit+1)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	page := movementsPage{Items: []movementResponse{}}
	if len(movements) > limit {
		movements = movements[:limit]
		next := movementCursor(movements[limit-1])
		page.NextCursor = &next
	}
	for _, movement := range movements {
		page.Items = append(page.Items, newMovementResponse(movement))
	}
	writeJSON(w, a.logger, http.StatusOK, page)
}
