package paylinks

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"gigjar/internal/database"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

type Status string

const (
	StatusOpen   Status = "open"
	StatusPaid   Status = "paid"
	StatusClosed Status = "closed"
)

const (
	referenceAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	referenceLength   = 10
	maxTitleLength    = 120
	maxDescLength     = 1000
	maxIdempotencyKey = 128
)

var (
	MaxLinkAmount = usdt.MustParse("100000")

	ErrNotFound               = errors.New("payment link not found")
	ErrIdempotencyKeyConflict = errors.New("idempotency key was already used with different link details")
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	return "validation failed"
}

type Link struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Reference   string
	Title       string
	Description string
	Amount      usdt.Amount
	Status      Status
	CreatedAt   time.Time
	PaidAt      *time.Time
	ClosedAt    *time.Time
}

type CreateInput struct {
	Title          string
	Description    string
	Amount         usdt.Amount
	IdempotencyKey string
}

type ListFilter struct {
	Status *Status
	Cursor string
	Limit  int
}

type Page struct {
	Links      []Link
	NextCursor string
}

type PublicInstructions struct {
	Link                 Link
	RecipientDisplayName string
	Network              tron.Network
	PayToAddress         *tron.Address
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

const linkColumns = `id, user_id, reference, title, description, amount, status, created_at, paid_at, closed_at`

func scanLink(row pgx.Row) (Link, error) {
	var (
		link   Link
		amount int64
	)
	err := row.Scan(&link.ID, &link.UserID, &link.Reference, &link.Title, &link.Description,
		&amount, &link.Status, &link.CreatedAt, &link.PaidAt, &link.ClosedAt)
	link.Amount = usdt.Amount(amount)
	return link, err
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, input CreateInput) (Link, bool, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if fields := validateCreate(input); len(fields) > 0 {
		return Link{}, false, &ValidationError{Fields: fields}
	}

	var idempotencyKey *string
	if input.IdempotencyKey != "" {
		idempotencyKey = &input.IdempotencyKey
	}

	for attempt := 0; attempt < 5; attempt++ {
		reference, err := newReference()
		if err != nil {
			return Link{}, false, err
		}
		link, err := scanLink(s.pool.QueryRow(ctx, `
			INSERT INTO payment_links (user_id, reference, title, description, amount, idempotency_key)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, idempotency_key) DO NOTHING
			RETURNING `+linkColumns,
			userID, reference, input.Title, input.Description, int64(input.Amount), idempotencyKey))
		switch {
		case err == nil:
			return link, true, nil
		case errors.Is(err, pgx.ErrNoRows):
			existing, err := s.byIdempotencyKey(ctx, userID, input.IdempotencyKey)
			if err != nil {
				return Link{}, false, err
			}
			if existing.Title != input.Title || existing.Description != input.Description || existing.Amount != input.Amount {
				return Link{}, false, ErrIdempotencyKeyConflict
			}
			return existing, false, nil
		case database.IsUniqueViolation(err, "payment_links_reference_unique"):
			continue
		default:
			return Link{}, false, fmt.Errorf("create payment link: %w", err)
		}
	}
	return Link{}, false, errors.New("could not allocate a unique payment reference")
}

func (s *Service) byIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (Link, error) {
	link, err := scanLink(s.pool.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM payment_links WHERE user_id = $1 AND idempotency_key = $2`,
		userID, key))
	if err != nil {
		return Link{}, fmt.Errorf("load idempotent payment link: %w", err)
	}
	return link, nil
}

func (s *Service) Get(ctx context.Context, userID, linkID uuid.UUID) (Link, error) {
	link, err := scanLink(s.pool.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM payment_links WHERE id = $1 AND user_id = $2`,
		linkID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if err != nil {
		return Link{}, fmt.Errorf("load payment link: %w", err)
	}
	return link, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, filter ListFilter) (Page, error) {
	limit := clampLimit(filter.Limit)
	cursor, err := decodeCursor(filter.Cursor)
	if err != nil {
		return Page{}, &ValidationError{Fields: map[string]string{"cursor": "is invalid"}}
	}
	var status *string
	if filter.Status != nil {
		value := string(*filter.Status)
		status = &value
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+linkColumns+` FROM payment_links
		WHERE user_id = $1
		  AND ($2::text IS NULL OR status = $2)
		  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4))
		ORDER BY created_at DESC, id DESC
		LIMIT $5`,
		userID, status, cursor.createdAt, cursor.id, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list payment links: %w", err)
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) {
		return scanLink(row)
	})
	if err != nil {
		return Page{}, fmt.Errorf("read payment links: %w", err)
	}
	page := Page{Links: links}
	if len(links) > limit {
		page.Links = links[:limit]
		last := page.Links[limit-1]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s *Service) Close(ctx context.Context, userID, linkID uuid.UUID) (Link, error) {
	link, err := scanLink(s.pool.QueryRow(ctx, `
		UPDATE payment_links SET status = 'closed', closed_at = now()
		WHERE id = $1 AND user_id = $2 AND status <> 'closed'
		RETURNING `+linkColumns,
		linkID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return s.Get(ctx, userID, linkID)
	}
	if err != nil {
		return Link{}, fmt.Errorf("close payment link: %w", err)
	}
	return link, nil
}

func (s *Service) PublicInstructions(ctx context.Context, reference string) (PublicInstructions, error) {
	var (
		instructions PublicInstructions
		amount       int64
		link         = &instructions.Link
	)
	err := s.pool.QueryRow(ctx, `
		SELECT pl.id, pl.user_id, pl.reference, pl.title, pl.description, pl.amount, pl.status,
		       pl.created_at, pl.paid_at, pl.closed_at, u.display_name, j.network, j.address
		FROM payment_links pl
		JOIN users u ON u.id = pl.user_id
		JOIN jars j ON j.user_id = pl.user_id
		WHERE pl.reference = $1`,
		strings.ToLower(strings.TrimSpace(reference)),
	).Scan(&link.ID, &link.UserID, &link.Reference, &link.Title, &link.Description, &amount, &link.Status,
		&link.CreatedAt, &link.PaidAt, &link.ClosedAt,
		&instructions.RecipientDisplayName, &instructions.Network, &instructions.PayToAddress)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicInstructions{}, ErrNotFound
	}
	if err != nil {
		return PublicInstructions{}, fmt.Errorf("load public payment instructions: %w", err)
	}
	link.Amount = usdt.Amount(amount)
	return instructions, nil
}

func ParseStatus(value string) (Status, bool) {
	switch status := Status(value); status {
	case StatusOpen, StatusPaid, StatusClosed:
		return status, true
	default:
		return "", false
	}
}

func validateCreate(input CreateInput) map[string]string {
	fields := map[string]string{}
	if length := utf8.RuneCountInString(input.Title); length < 1 || length > maxTitleLength {
		fields["title"] = fmt.Sprintf("must be between 1 and %d characters", maxTitleLength)
	}
	if utf8.RuneCountInString(input.Description) > maxDescLength {
		fields["description"] = fmt.Sprintf("must be at most %d characters", maxDescLength)
	}
	if !input.Amount.IsPositive() {
		fields["amount"] = "must be greater than zero"
	} else if input.Amount > MaxLinkAmount {
		fields["amount"] = "must be at most " + MaxLinkAmount.String()
	}
	if len(input.IdempotencyKey) > maxIdempotencyKey {
		fields["idempotency_key"] = fmt.Sprintf("must be at most %d characters", maxIdempotencyKey)
	}
	return fields
}

func newReference() (string, error) {
	random := make([]byte, referenceLength)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate payment reference: %w", err)
	}
	reference := make([]byte, referenceLength)
	for index, value := range random {
		reference[index] = referenceAlphabet[int(value)%len(referenceAlphabet)]
	}
	return string(reference), nil
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 25
	case limit > 100:
		return 100
	default:
		return limit
	}
}
