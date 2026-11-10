package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/database"
	"gigjar/internal/jars"
	"gigjar/internal/ledger"
	"gigjar/internal/tron"
)

var (
	ErrEmailTaken         = errors.New("an account with this email already exists")
	ErrInvalidCredentials = errors.New("email or password is incorrect")
	ErrUserNotFound       = errors.New("user not found")
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "validation failed"
}

type User struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
	CreatedAt   time.Time
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
}

type Config struct {
	PasswordParams PasswordParams
	SessionTTL     time.Duration
	Network        tron.Network
}

type Service struct {
	pool          *pgxpool.Pool
	config        Config
	decoyPassword string
}

func NewService(pool *pgxpool.Pool, config Config) (*Service, error) {
	decoy, err := HashPassword(uuid.NewString(), config.PasswordParams)
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, config: config, decoyPassword: decoy}, nil
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (User, Session, error) {
	email, displayName, fields := validateRegistration(input)
	if len(fields) > 0 {
		return User{}, Session{}, &ValidationError{Fields: fields}
	}
	passwordHash, err := HashPassword(input.Password, s.config.PasswordParams)
	if err != nil {
		return User{}, Session{}, err
	}

	var (
		user    User
		session Session
	)
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3)
			RETURNING id, email, display_name, created_at`,
			email, passwordHash, displayName,
		).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
		if database.IsUniqueViolation(err, "users_email_unique") {
			return ErrEmailTaken
		}
		if err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		if err := jars.Create(ctx, tx, user.ID, s.config.Network); err != nil {
			return err
		}
		if err := ledger.OpenUserAccounts(ctx, tx, user.ID); err != nil {
			return err
		}
		session, err = createSession(ctx, tx, user.ID, s.config.SessionTTL)
		return err
	})
	if err != nil {
		return User{}, Session{}, err
	}
	return user, session, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (User, Session, error) {
	var (
		user         User
		passwordHash *string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, display_name, created_at, password_hash FROM users WHERE email = $1`,
		strings.TrimSpace(email),
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && passwordHash == nil) {
		_, _ = VerifyPassword(password, s.decoyPassword)
		return User{}, Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("load user: %w", err)
	}
	matches, err := VerifyPassword(password, *passwordHash)
	if err != nil {
		return User{}, Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !matches {
		return User{}, Session{}, ErrInvalidCredentials
	}
	session, err := createSession(ctx, s.pool, user.ID, s.config.SessionTTL)
	if err != nil {
		return User{}, Session{}, err
	}
	return user, session, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	userID, err := userIDForSession(ctx, s.pool, token)
	if err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, userID)
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return revokeSession(ctx, s.pool, token)
}

func (s *Service) UserByID(ctx context.Context, userID uuid.UUID) (User, error) {
	var user User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, display_name, created_at FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load user: %w", err)
	}
	return user, nil
}

func (s *Service) UpdateDisplayName(ctx context.Context, userID uuid.UUID, displayName string) (User, error) {
	trimmed := strings.TrimSpace(displayName)
	if problem := validateDisplayName(trimmed); problem != "" {
		return User{}, &ValidationError{Fields: map[string]string{"display_name": problem}}
	}
	var user User
	err := s.pool.QueryRow(ctx, `
		UPDATE users SET display_name = $2 WHERE id = $1
		RETURNING id, email, display_name, created_at`,
		userID, trimmed,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("update display name: %w", err)
	}
	return user, nil
}

func validateRegistration(input RegisterInput) (string, string, map[string]string) {
	fields := map[string]string{}
	email := strings.TrimSpace(input.Email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		fields["email"] = "must be a valid email address"
	}
	passwordLength := utf8.RuneCountInString(input.Password)
	if passwordLength < MinPasswordLength || passwordLength > MaxPasswordLength {
		fields["password"] = fmt.Sprintf("must be between %d and %d characters", MinPasswordLength, MaxPasswordLength)
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if problem := validateDisplayName(displayName); problem != "" {
		fields["display_name"] = problem
	}
	return email, displayName, fields
}

func validateDisplayName(displayName string) string {
	length := utf8.RuneCountInString(displayName)
	if length < 1 || length > maxDisplayNameLength {
		return fmt.Sprintf("must be between 1 and %d characters", maxDisplayNameLength)
	}
	return ""
}
