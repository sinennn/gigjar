package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"gigjar/internal/identity"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/paylinks"
)

const maxRequestBodyBytes = 64 << 10

var errMalformedBody = errors.New("malformed request body")

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errMalformedBody
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errMalformedBody
	}
	return nil
}

func queryLimit(r *http.Request) (int, bool) {
	text := r.URL.Query().Get("limit")
	if text == "" {
		return 25, true
	}
	limit, err := strconv.Atoi(text)
	if err != nil || limit < 1 || limit > 100 {
		return 0, false
	}
	return limit, true
}

func (a *api) respondError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		identityValidation *identity.ValidationError
		paylinkValidation  *paylinks.ValidationError
	)
	switch {
	case errors.Is(err, errMalformedBody):
		writeError(w, r, a.logger, http.StatusBadRequest, "malformed_body", "request body must be a single JSON object with known fields")
	case errors.As(err, &identityValidation):
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid", identityValidation.Fields)
	case errors.As(err, &paylinkValidation):
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid", paylinkValidation.Fields)
	case errors.Is(err, identity.ErrEmailTaken):
		writeError(w, r, a.logger, http.StatusConflict, "email_taken", err.Error())
	case errors.Is(err, identity.ErrInvalidCredentials):
		writeError(w, r, a.logger, http.StatusUnauthorized, "invalid_credentials", err.Error())
	case errors.Is(err, identity.ErrSessionInvalid), errors.Is(err, identity.ErrUserNotFound):
		writeError(w, r, a.logger, http.StatusUnauthorized, "unauthenticated", "sign in to continue")
	case errors.Is(err, paylinks.ErrNotFound):
		writeError(w, r, a.logger, http.StatusNotFound, "payment_link_not_found", err.Error())
	case errors.Is(err, paylinks.ErrIdempotencyKeyConflict):
		writeError(w, r, a.logger, http.StatusConflict, "idempotency_key_conflict", err.Error())
	case errors.Is(err, jars.ErrDestinationNotFound):
		writeError(w, r, a.logger, http.StatusNotFound, "cleva_destination_not_set", err.Error())
	case errors.Is(err, jars.ErrJarNotFound):
		writeError(w, r, a.logger, http.StatusNotFound, "jar_not_found", err.Error())
	case errors.Is(err, ledger.ErrInsufficientFunds):
		writeError(w, r, a.logger, http.StatusConflict, "insufficient_funds", err.Error())
	default:
		a.logger.Error("request failed", "request_id", requestID(r), "error", err)
		writeError(w, r, a.logger, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}
