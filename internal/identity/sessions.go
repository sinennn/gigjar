package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/database"
)

var ErrSessionInvalid = errors.New("session is invalid or expired")

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Token     string
	ExpiresAt time.Time
}

func newSessionToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashSessionToken(token), nil
}

func hashSessionToken(token string) []byte {
	digest := sha256.Sum256([]byte(token))
	return digest[:]
}

func createSession(ctx context.Context, db database.Querier, userID uuid.UUID, ttl time.Duration) (Session, error) {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return Session{}, err
	}
	session := Session{UserID: userID, Token: token}
	err = db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, now() + make_interval(secs => $3))
		RETURNING id, expires_at`,
		userID, tokenHash, ttl.Seconds(),
	).Scan(&session.ID, &session.ExpiresAt)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func userIDForSession(ctx context.Context, db database.Querier, token string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT user_id FROM sessions
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		hashSessionToken(token),
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrSessionInvalid
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("look up session: %w", err)
	}
	return userID, nil
}

func revokeSession(ctx context.Context, db database.Querier, token string) error {
	_, err := db.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		hashSessionToken(token))
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
