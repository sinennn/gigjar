package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/database"
)

const PasswordResetTTL = 30 * time.Minute

var ErrResetTokenInvalid = errors.New("this reset link is invalid, already used or expired")

type PasswordResetRequest struct {
	User  User
	Token string
}

func (s *Service) RequestPasswordReset(ctx context.Context, email string) (PasswordResetRequest, bool, error) {
	var request PasswordResetRequest
	found := false
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT id, email, display_name, created_at FROM users WHERE email = $1 FOR UPDATE`,
			strings.TrimSpace(email),
		).Scan(&request.User.ID, &request.User.Email, &request.User.DisplayName, &request.User.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find user for reset: %w", err)
		}
		found = true
		if _, err := tx.Exec(ctx,
			`UPDATE password_resets SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`, request.User.ID); err != nil {
			return fmt.Errorf("invalidate older reset links: %w", err)
		}
		token, tokenHash, err := newSessionToken()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO password_resets (user_id, token_hash, expires_at)
			VALUES ($1, $2, now() + make_interval(secs => $3))`,
			request.User.ID, tokenHash, PasswordResetTTL.Seconds()); err != nil {
			return fmt.Errorf("store reset link: %w", err)
		}
		request.Token = token
		return nil
	})
	if err != nil {
		return PasswordResetRequest{}, false, err
	}
	return request, found, nil
}

func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) (User, Session, error) {
	if length := utf8.RuneCountInString(newPassword); length < MinPasswordLength || length > MaxPasswordLength {
		return User{}, Session{}, &ValidationError{Fields: map[string]string{
			"password": fmt.Sprintf("must be between %d and %d characters", MinPasswordLength, MaxPasswordLength),
		}}
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return User{}, Session{}, ErrResetTokenInvalid
	}
	passwordHash, err := HashPassword(newPassword, s.config.PasswordParams)
	if err != nil {
		return User{}, Session{}, err
	}

	var (
		user    User
		session Session
	)
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			resetID int64
			userID  uuid.UUID
		)
		err := tx.QueryRow(ctx, `
			SELECT id, user_id FROM password_resets
			WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
			FOR UPDATE`,
			hashSessionToken(token),
		).Scan(&resetID, &userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrResetTokenInvalid
		}
		if err != nil {
			return fmt.Errorf("find reset link: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE password_resets SET used_at = now() WHERE id = $1`, resetID); err != nil {
			return fmt.Errorf("use reset link: %w", err)
		}
		err = tx.QueryRow(ctx, `
			UPDATE users SET password_hash = $2 WHERE id = $1
			RETURNING id, email, display_name, created_at`,
			userID, passwordHash,
		).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
		if err != nil {
			return fmt.Errorf("set new password: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
			return fmt.Errorf("sign out other sessions: %w", err)
		}
		session, err = createSession(ctx, tx, userID, s.config.SessionTTL)
		return err
	})
	if err != nil {
		return User{}, Session{}, err
	}
	return user, session, nil
}
