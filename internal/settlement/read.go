package settlement

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gigjar/internal/database"
)

var ErrInvalidCursor = errors.New("invalid cursor")

type Page struct {
	Settlements []Settlement
	NextCursor  string
}

func List(ctx context.Context, db database.Querier, userID uuid.UUID, cursor string, limit int) (Page, error) {
	if limit <= 0 {
		limit = 25
	}
	limit = min(limit, 100)
	createdAt, cursorID, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	rows, err := db.Query(ctx, `
		SELECT `+settlementColumns+` FROM settlements
		WHERE user_id = $1 AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3))
		ORDER BY created_at DESC, id DESC
		LIMIT $4`,
		userID, createdAt, cursorID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list settlements: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Settlement, error) { return scanSettlement(row) })
	if err != nil {
		return Page{}, fmt.Errorf("read settlements: %w", err)
	}
	page := Page{Settlements: items}
	if len(items) > limit {
		page.Settlements = items[:limit]
		last := page.Settlements[limit-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(last.CreatedAt.UnixMicro(), 10) + "|" + last.ID.String()))
	}
	return page, nil
}

func Get(ctx context.Context, db database.Querier, id uuid.UUID) (Settlement, error) {
	return scanSettlement(db.QueryRow(ctx, `SELECT `+settlementColumns+` FROM settlements WHERE id = $1`, id))
}

func decodeCursor(cursor string) (*time.Time, uuid.UUID, error) {
	if cursor == "" {
		return nil, uuid.Nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	micros, idText, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	value, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return nil, uuid.Nil, ErrInvalidCursor
	}
	createdAt := time.UnixMicro(value)
	return &createdAt, id, nil
}
