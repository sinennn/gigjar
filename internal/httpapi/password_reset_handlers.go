package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gigjar/internal/identity"
	"gigjar/internal/mail"
)

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type acceptedResponse struct {
	Status string `json:"status"`
}

func (a *api) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var request forgotPasswordRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if len(email) < 3 || len(email) > 254 || !strings.Contains(email, "@") {
		writeErrorWithFields(w, r, a.logger, http.StatusUnprocessableEntity, "validation_failed", "some fields are invalid",
			map[string]string{"email": "must be a valid email address"})
		return
	}
	if !a.resetAttempts.allow(email) {
		writeError(w, r, a.logger, http.StatusTooManyRequests, "too_many_attempts", "too many reset requests for this email, try again later")
		return
	}
	go a.sendPasswordReset(requestID(r), email)
	writeJSON(w, a.logger, http.StatusAccepted, acceptedResponse{Status: "if an account exists for that email, a reset link is on its way"})
}

func (a *api) sendPasswordReset(requestID, email string) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	request, found, err := a.identity.RequestPasswordReset(ctx, email)
	if err != nil {
		a.logger.Error("password reset request failed", "request_id", requestID, "error", err)
		return
	}
	if !found {
		return
	}
	link := a.publicWebURL + "/reset-password?token=" + url.QueryEscape(request.Token)
	err = a.mailer.Send(ctx, mail.Message{
		To:      request.User.Email,
		Subject: "Reset your GigJar password",
		Text: fmt.Sprintf("Hi %s,\n\nSomeone (hopefully you) asked to reset the password for your GigJar account.\n\n"+
			"Choose a new password here. The link works once, for the next %d minutes:\n%s\n\n"+
			"If you didn't ask for this, you can ignore this email and your password stays the same.\n\nGigJar\n",
			request.User.DisplayName, int(identity.PasswordResetTTL.Minutes()), link),
	})
	if err != nil && !errors.Is(err, mail.ErrNotConfigured) {
		a.logger.Error("password reset email failed", "request_id", requestID, "user_id", request.User.ID, "error", err)
		return
	}
	if err == nil && a.mailer.Configured() {
		a.logger.Info("password reset email sent", "request_id", requestID, "user_id", request.User.ID)
	}
}

func (a *api) resetPassword(w http.ResponseWriter, r *http.Request) {
	var request resetPasswordRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	user, session, err := a.identity.ResetPassword(r.Context(), request.Token, request.Password)
	if errors.Is(err, identity.ErrResetTokenInvalid) {
		writeError(w, r, a.logger, http.StatusBadRequest, "reset_token_invalid", err.Error())
		return
	}
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, a.logger, http.StatusOK, newUserResponse(user))
}
