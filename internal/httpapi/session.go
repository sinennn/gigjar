package httpapi

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"gigjar/internal/identity"
)

const sessionCookieName = "gigjar_session"

type contextKey int

const currentUserKey contextKey = iota

func requestID(r *http.Request) string {
	return middleware.GetReqID(r.Context())
}

func currentUser(r *http.Request) identity.User {
	user, _ := r.Context().Value(currentUserKey).(identity.User)
	return user
}

func (a *api) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, r, a.logger, http.StatusUnauthorized, "unauthenticated", "sign in to continue")
			return
		}
		user, err := a.identity.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			a.respondError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), currentUserKey, user)))
	})
}

func (a *api) setSessionCookie(w http.ResponseWriter, session identity.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *api) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

type attemptLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	attempts map[string][]time.Time
	now      func() time.Time
}

func newAttemptLimiter(limit int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{limit: limit, window: window, attempts: map[string][]time.Time{}, now: time.Now}
}

func (l *attemptLimiter) allow(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-l.window)
	for existing, times := range l.attempts {
		if len(times) == 0 || times[len(times)-1].Before(cutoff) {
			delete(l.attempts, existing)
		}
	}
	recent := l.attempts[normalized][:0]
	for _, attempt := range l.attempts[normalized] {
		if attempt.After(cutoff) {
			recent = append(recent, attempt)
		}
	}
	if len(recent) >= l.limit {
		l.attempts[normalized] = recent
		return false
	}
	l.attempts[normalized] = append(recent, now)
	return true
}
