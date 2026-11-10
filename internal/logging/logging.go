package logging

import (
	"io"
	"log/slog"
	"strings"
)

const redactedValue = "[redacted]"

var sensitiveKeyFragments = []string{
	"password",
	"secret",
	"token",
	"seed",
	"mnemonic",
	"private_key",
	"privatekey",
	"api_key",
	"apikey",
	"authorization",
	"cookie",
	"database_url",
}

func New(output io.Writer, level, format string) *slog.Logger {
	options := &slog.HandlerOptions{
		Level:       parseLevel(level),
		ReplaceAttr: redactSensitive,
	}
	if format == "text" {
		return slog.New(slog.NewTextHandler(output, options))
	}
	return slog.New(slog.NewJSONHandler(output, options))
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func redactSensitive(_ []string, attr slog.Attr) slog.Attr {
	if IsSensitiveKey(attr.Key) {
		return slog.String(attr.Key, redactedValue)
	}
	return attr
}

func IsSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, fragment := range sensitiveKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
