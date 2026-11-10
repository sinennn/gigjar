package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"gigjar/internal/googleauth"
	"gigjar/internal/identity"
)

const oauthFlowCookie = "gigjar_oauth"

type providersResponse struct {
	Google bool `json:"google"`
	AI     bool `json:"ai"`
}

func (a *api) getProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.logger, http.StatusOK, providersResponse{Google: a.google != nil, AI: a.ai.Enabled()})
}

func (a *api) startGoogleSignIn(w http.ResponseWriter, r *http.Request) {
	if a.google == nil {
		writeError(w, r, a.logger, http.StatusNotFound, "google_sign_in_unavailable", "Google sign-in is not configured")
		return
	}
	authURL, flow, err := a.google.Begin(r.Context(), r.URL.Query().Get("return_to"))
	if err != nil {
		a.logger.Error("google sign-in could not start", "request_id", requestID(r), "error", err)
		http.Redirect(w, r, a.webURL("/login", "google_unavailable"), http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthFlowCookie,
		Value:    flow,
		Path:     "/",
		MaxAge:   int((10 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (a *api) finishGoogleSignIn(w http.ResponseWriter, r *http.Request) {
	if a.google == nil {
		writeError(w, r, a.logger, http.StatusNotFound, "google_sign_in_unavailable", "Google sign-in is not configured")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthFlowCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")

	query := r.URL.Query()
	if query.Get("error") != "" {
		http.Redirect(w, r, a.webURL("/login", "google_cancelled"), http.StatusFound)
		return
	}
	cookie, err := r.Cookie(oauthFlowCookie)
	if err != nil {
		http.Redirect(w, r, a.webURL("/login", "google_expired"), http.StatusFound)
		return
	}
	external, returnTo, err := a.google.Complete(r.Context(), cookie.Value, query.Get("state"), query.Get("code"))
	if err != nil {
		a.logger.Warn("google sign-in rejected", "request_id", requestID(r), "error", err)
		reason := "google_failed"
		if errors.Is(err, googleauth.ErrInvalidFlow) {
			reason = "google_expired"
		}
		http.Redirect(w, r, a.webURL("/login", reason), http.StatusFound)
		return
	}
	result, err := a.identity.SignInWithExternal(r.Context(), external)
	if errors.Is(err, identity.ErrUnverifiedEmail) {
		http.Redirect(w, r, a.webURL("/login", "google_email_unverified"), http.StatusFound)
		return
	}
	if err != nil {
		a.logger.Error("google sign-in failed", "request_id", requestID(r), "error", err)
		http.Redirect(w, r, a.webURL("/login", "google_failed"), http.StatusFound)
		return
	}
	a.logger.Info("google sign-in", "request_id", requestID(r), "user_id", result.User.ID, "created", result.Created, "linked", result.Linked)
	a.setSessionCookie(w, result.Session)
	http.Redirect(w, r, a.publicWebURL+googleauth.SafeReturnPath(returnTo), http.StatusFound)
}

func (a *api) webURL(path, errorCode string) string {
	target := a.publicWebURL + path
	if errorCode != "" {
		target += "?error=" + url.QueryEscape(errorCode)
	}
	return target
}
