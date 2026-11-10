package paylinks

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type pageCursor struct {
	createdAt *time.Time
	id        uuid.UUID
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(createdAt.UnixMicro(), 10) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (pageCursor, error) {
	if cursor == "" {
		return pageCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return pageCursor{}, err
	}
	micros, idText, found := strings.Cut(string(raw), "|")
	if !found {
		return pageCursor{}, errors.New("malformed cursor")
	}
	unixMicro, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return pageCursor{}, err
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return pageCursor{}, err
	}
	createdAt := time.UnixMicro(unixMicro).UTC()
	return pageCursor{createdAt: &createdAt, id: id}, nil
}
