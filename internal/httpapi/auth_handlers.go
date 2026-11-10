package httpapi

import (
	"net/http"

	"gigjar/internal/identity"
)

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateProfileRequest struct {
	DisplayName *string `json:"display_name"`
}

func (a *api) register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	user, session, err := a.identity.Register(r.Context(), identity.RegisterInput{
		Email:       request.Email,
		Password:    request.Password,
		DisplayName: request.DisplayName,
	})
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, a.logger, http.StatusCreated, newUserResponse(user))
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	if !a.loginAttempts.allow(request.Email) {
		writeError(w, r, a.logger, http.StatusTooManyRequests, "too_many_attempts", "too many sign-in attempts, try again in a minute")
		return
	}
	user, session, err := a.identity.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		a.respondError(w, r, err)
		return
	}
	a.setSessionCookie(w, session)
	writeJSON(w, a.logger, http.StatusOK, newUserResponse(user))
}

func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if err := a.identity.Logout(r.Context(), cookie.Value); err != nil {
			a.respondError(w, r, err)
			return
		}
	}
	a.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) getMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.logger, http.StatusOK, newUserResponse(currentUser(r)))
}

func (a *api) updateMe(w http.ResponseWriter, r *http.Request) {
	var request updateProfileRequest
	if err := decodeJSON(w, r, &request); err != nil {
		a.respondError(w, r, err)
		return
	}
	user := currentUser(r)
	if request.DisplayName != nil {
		updated, err := a.identity.UpdateDisplayName(r.Context(), user.ID, *request.DisplayName)
		if err != nil {
			a.respondError(w, r, err)
			return
		}
		user = updated
	}
	writeJSON(w, a.logger, http.StatusOK, newUserResponse(user))
}
