package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
)

const (
	ProviderGoogle       = "google"
	maxDisplayNameLength = 80
)

var ErrUnverifiedEmail = errors.New("the Google account's email address is not verified")

type ExternalIdentity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

type ExternalSignIn struct {
	User    User
	Session Session
	Created bool
	Linked  bool
}

func (s *Service) SignInWithExternal(ctx context.Context, identity ExternalIdentity) (ExternalSignIn, error) {
	identity.Email = strings.ToLower(strings.TrimSpace(identity.Email))
	if identity.Provider != ProviderGoogle || identity.Subject == "" || len(identity.Subject) > 255 {
		return ExternalSignIn{}, errors.New("invalid external identity")
	}
	if !identity.EmailVerified || !strings.Contains(identity.Email, "@") {
		return ExternalSignIn{}, ErrUnverifiedEmail
	}

	var result ExternalSignIn
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var userID uuid.UUID
		err := tx.QueryRow(ctx, `
			UPDATE user_identities SET last_login_at = now()
			WHERE provider = $1 AND subject = $2
			RETURNING user_id`,
			identity.Provider, identity.Subject,
		).Scan(&userID)
		switch {
		case err == nil:
		case errors.Is(err, pgx.ErrNoRows):
			userID, result.Created, err = s.userForNewIdentity(ctx, tx, identity)
			if err != nil {
				return err
			}
			result.Linked = !result.Created
			_, err = tx.Exec(ctx, `
				INSERT INTO user_identities (user_id, provider, subject, email) VALUES ($1, $2, $3, $4)`,
				userID, identity.Provider, identity.Subject, identity.Email)
			if database.IsUniqueViolation(err, "user_identities_user_provider_unique") {
				return fmt.Errorf("account %s is already linked to a different %s identity", userID, identity.Provider)
			}
			if err != nil {
				return fmt.Errorf("link identity: %w", err)
			}
		default:
			return fmt.Errorf("find identity: %w", err)
		}

		err = tx.QueryRow(ctx, `SELECT id, email, display_name, created_at FROM users WHERE id = $1`, userID).
			Scan(&result.User.ID, &result.User.Email, &result.User.DisplayName, &result.User.CreatedAt)
		if err != nil {
			return fmt.Errorf("load user: %w", err)
		}
		result.Session, err = createSession(ctx, tx, userID, s.config.SessionTTL)
		return err
	})
	if err != nil {
		return ExternalSignIn{}, err
	}
	return result, nil
}

func (s *Service) userForNewIdentity(ctx context.Context, tx pgx.Tx, identity ExternalIdentity) (uuid.UUID, bool, error) {
	var userID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1 FOR UPDATE`, identity.Email).Scan(&userID)
	if err == nil {
		return userID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("find user by email: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name) VALUES ($1, NULL, $2)
		RETURNING id`,
		identity.Email, externalDisplayName(identity),
	).Scan(&userID)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("create user: %w", err)
	}
	if err := jars.Create(ctx, tx, userID, s.config.Network); err != nil {
		return uuid.Nil, false, err
	}
	if err := ledger.OpenUserAccounts(ctx, tx, userID); err != nil {
		return uuid.Nil, false, err
	}
	return userID, true, nil
}

func externalDisplayName(identity ExternalIdentity) string {
	name := strings.Join(strings.Fields(identity.Name), " ")
	if name == "" {
		name, _, _ = strings.Cut(identity.Email, "@")
	}
	if utf8.RuneCountInString(name) > maxDisplayNameLength {
		name = string([]rune(name)[:maxDisplayNameLength])
	}
	if name == "" {
		name = "GigJar student"
	}
	return name
}
