package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var minimumSettlementThreshold = usdt.MustParse("10.000001")

type Environment string

const (
	EnvironmentDevelopment Environment = "development"
	EnvironmentTest        Environment = "test"
	EnvironmentProduction  Environment = "production"
)

type LookupFunc func(key string) (string, bool)

type Common struct {
	Environment Environment
	LogLevel    string
	LogFormat   string
	DatabaseURL string
	Network     tron.Network
}

type API struct {
	Common
	HTTPAddr            string
	CORSAllowedOrigins  []string
	ReadHeaderTimeout   time.Duration
	RequestTimeout      time.Duration
	ShutdownTimeout     time.Duration
	PublicWebURL        string
	SessionTTL          time.Duration
	SecureCookies       bool
	SettlementThreshold usdt.Amount
}

type Migrate struct {
	Common
}

func (e Environment) IsProduction() bool {
	return e == EnvironmentProduction
}

func LoadAPI(lookup LookupFunc) (API, error) {
	reader := newReader(lookup)
	cfg := API{
		Common:              readCommon(reader),
		HTTPAddr:            reader.string("GIGJAR_HTTP_ADDR", ":8080"),
		CORSAllowedOrigins:  reader.list("GIGJAR_CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		ReadHeaderTimeout:   reader.duration("GIGJAR_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		RequestTimeout:      reader.duration("GIGJAR_HTTP_REQUEST_TIMEOUT", 15*time.Second),
		ShutdownTimeout:     reader.duration("GIGJAR_SHUTDOWN_TIMEOUT", 20*time.Second),
		PublicWebURL:        strings.TrimRight(reader.string("GIGJAR_PUBLIC_WEB_URL", "http://localhost:3000"), "/"),
		SessionTTL:          reader.duration("GIGJAR_SESSION_TTL", 30*24*time.Hour),
		SettlementThreshold: reader.amount("GIGJAR_SETTLEMENT_THRESHOLD", usdt.MustParse("11")),
	}
	cfg.SecureCookies = reader.boolean("GIGJAR_SECURE_COOKIES", cfg.Environment.IsProduction())
	reader.requirePositive("GIGJAR_SESSION_TTL", cfg.SessionTTL)
	if cfg.SettlementThreshold < minimumSettlementThreshold {
		reader.fail("GIGJAR_SETTLEMENT_THRESHOLD", "must be above Cleva's 10 USDT minimum deposit")
	}
	if parsed, err := url.Parse(cfg.PublicWebURL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		reader.fail("GIGJAR_PUBLIC_WEB_URL", "must be an absolute URL")
	}
	if cfg.Environment.IsProduction() && !cfg.SecureCookies {
		reader.fail("GIGJAR_SECURE_COOKIES", "must be true in production")
	}
	reader.requirePositive("GIGJAR_HTTP_READ_HEADER_TIMEOUT", cfg.ReadHeaderTimeout)
	reader.requirePositive("GIGJAR_HTTP_REQUEST_TIMEOUT", cfg.RequestTimeout)
	reader.requirePositive("GIGJAR_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout)
	if cfg.Environment.IsProduction() {
		for _, origin := range cfg.CORSAllowedOrigins {
			if origin == "*" {
				reader.fail("GIGJAR_CORS_ALLOWED_ORIGINS", "wildcard origin is not allowed in production")
			}
		}
	}
	return cfg, reader.err()
}

func LoadMigrate(lookup LookupFunc) (Migrate, error) {
	reader := newReader(lookup)
	cfg := Migrate{Common: readCommon(reader)}
	return cfg, reader.err()
}

func readCommon(reader *reader) Common {
	common := Common{
		Environment: Environment(reader.string("GIGJAR_ENV", string(EnvironmentDevelopment))),
		LogLevel:    strings.ToLower(reader.string("GIGJAR_LOG_LEVEL", "info")),
		LogFormat:   strings.ToLower(reader.string("GIGJAR_LOG_FORMAT", "json")),
		DatabaseURL: reader.required("GIGJAR_DATABASE_URL"),
	}
	network, err := tron.ParseNetwork(reader.string("GIGJAR_CHAIN_NETWORK", string(tron.NetworkSimulated)))
	if err != nil {
		reader.fail("GIGJAR_CHAIN_NETWORK", err.Error())
	}
	common.Network = network
	if common.Environment == EnvironmentProduction && network.IsSimulated() {
		reader.fail("GIGJAR_CHAIN_NETWORK", "the simulated network is not allowed in production")
	}
	switch common.Environment {
	case EnvironmentDevelopment, EnvironmentTest, EnvironmentProduction:
	default:
		reader.fail("GIGJAR_ENV", fmt.Sprintf("must be one of development, test, production; got %q", common.Environment))
	}
	switch common.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		reader.fail("GIGJAR_LOG_LEVEL", fmt.Sprintf("must be one of debug, info, warn, error; got %q", common.LogLevel))
	}
	switch common.LogFormat {
	case "json", "text":
	default:
		reader.fail("GIGJAR_LOG_FORMAT", fmt.Sprintf("must be json or text; got %q", common.LogFormat))
	}
	return common
}

type reader struct {
	lookup LookupFunc
	errs   []error
}

func newReader(lookup LookupFunc) *reader {
	return &reader{lookup: lookup}
}

func (r *reader) fail(key, reason string) {
	r.errs = append(r.errs, fmt.Errorf("%s: %s", key, reason))
}

func (r *reader) err() error {
	return errors.Join(r.errs...)
}

func (r *reader) raw(key string) (string, bool) {
	value, ok := r.lookup(key)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func (r *reader) string(key, fallback string) string {
	if value, ok := r.raw(key); ok {
		return value
	}
	return fallback
}

func (r *reader) required(key string) string {
	value, ok := r.raw(key)
	if !ok {
		r.fail(key, "is required")
	}
	return value
}

func (r *reader) list(key string, fallback []string) []string {
	value, ok := r.raw(key)
	if !ok {
		return fallback
	}
	var items []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func (r *reader) duration(key string, fallback time.Duration) time.Duration {
	value, ok := r.raw(key)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		r.fail(key, fmt.Sprintf("invalid duration %q", value))
		return fallback
	}
	return parsed
}

func (r *reader) amount(key string, fallback usdt.Amount) usdt.Amount {
	value, ok := r.raw(key)
	if !ok {
		return fallback
	}
	parsed, err := usdt.Parse(value)
	if err != nil {
		r.fail(key, fmt.Sprintf("invalid USDT amount %q", value))
		return fallback
	}
	return parsed
}

func (r *reader) boolean(key string, fallback bool) bool {
	value, ok := r.raw(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		r.fail(key, fmt.Sprintf("invalid boolean %q", value))
		return fallback
	}
}

func (r *reader) requirePositive(key string, value time.Duration) {
	if value <= 0 {
		r.fail(key, "must be positive")
	}
}
