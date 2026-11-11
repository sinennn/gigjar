package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gigjar/internal/chain/trongrid"
	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

var clevaMinimumDeposit = usdt.MustParse("10")

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
	SettlementFee       usdt.Amount
	GoogleClientID      string
	GoogleClientSecret  string
	GoogleRedirectURL   string
	GroqAPIKey          string
	GroqModel           string
	SMTPHost            string
	SMTPPort            int
	SMTPUsername        string
	SMTPPassword        string
	MailFrom            string
}

func (a API) SMTPEnabled() bool {
	return a.SMTPHost != ""
}

func (a API) GoogleEnabled() bool {
	return a.GoogleClientID != "" && a.GoogleClientSecret != ""
}

type Migrate struct {
	Common
}

type Watcher struct {
	Common
	TronGridURL        string
	TronGridAPIKey     string
	Tick               time.Duration
	BatchSize          int
	ActivePollInterval time.Duration
	IdlePollInterval   time.Duration
	CursorOverlap      time.Duration
	DropAfter          time.Duration
	ReconcileInterval  time.Duration
}

func (e Environment) IsProduction() bool {
	return e == EnvironmentProduction
}

func LoadAPI(lookup LookupFunc) (API, error) {
	reader := newReader(lookup)
	cfg := API{
		Common:             readCommon(reader),
		HTTPAddr:           reader.string("GIGJAR_HTTP_ADDR", ":8080"),
		CORSAllowedOrigins: reader.list("GIGJAR_CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		ReadHeaderTimeout:  reader.duration("GIGJAR_HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
		RequestTimeout:     reader.duration("GIGJAR_HTTP_REQUEST_TIMEOUT", 15*time.Second),
		ShutdownTimeout:    reader.duration("GIGJAR_SHUTDOWN_TIMEOUT", 20*time.Second),
		PublicWebURL:       strings.TrimRight(reader.string("GIGJAR_PUBLIC_WEB_URL", "http://localhost:3000"), "/"),
		SessionTTL:         reader.duration("GIGJAR_SESSION_TTL", 30*24*time.Hour),
	}
	cfg.SettlementThreshold, cfg.SettlementFee = readSettlement(reader)
	cfg.GoogleClientID = reader.string("GIGJAR_GOOGLE_CLIENT_ID", "")
	cfg.GoogleClientSecret = reader.string("GIGJAR_GOOGLE_CLIENT_SECRET", "")
	cfg.GoogleRedirectURL = reader.string("GIGJAR_GOOGLE_REDIRECT_URL", cfg.PublicWebURL+"/api/v1/auth/google/callback")
	if (cfg.GoogleClientID == "") != (cfg.GoogleClientSecret == "") {
		reader.fail("GIGJAR_GOOGLE_CLIENT_ID", "and GIGJAR_GOOGLE_CLIENT_SECRET must be set together")
	}
	if parsed, err := url.Parse(cfg.GoogleRedirectURL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		reader.fail("GIGJAR_GOOGLE_REDIRECT_URL", "must be an absolute URL")
	}
	cfg.GroqAPIKey = reader.string("GIGJAR_GROQ_API_KEY", "")
	cfg.GroqModel = reader.string("GIGJAR_GROQ_MODEL", "openai/gpt-oss-20b")
	cfg.SMTPHost = reader.string("GIGJAR_SMTP_HOST", "")
	cfg.SMTPPort = reader.integer("GIGJAR_SMTP_PORT", 587)
	cfg.SMTPUsername = reader.string("GIGJAR_SMTP_USERNAME", "")
	cfg.SMTPPassword = reader.string("GIGJAR_SMTP_PASSWORD", "")
	cfg.MailFrom = reader.string("GIGJAR_MAIL_FROM", "")
	if cfg.SMTPHost != "" {
		if cfg.MailFrom == "" {
			reader.fail("GIGJAR_MAIL_FROM", "is required when GIGJAR_SMTP_HOST is set")
		}
		if cfg.SMTPPort <= 0 || cfg.SMTPPort > 65535 {
			reader.fail("GIGJAR_SMTP_PORT", "must be a valid port")
		}
	}
	cfg.SecureCookies = reader.boolean("GIGJAR_SECURE_COOKIES", cfg.Environment.IsProduction())
	reader.requirePositive("GIGJAR_SESSION_TTL", cfg.SessionTTL)
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

type Sweeper struct {
	Common
	SettlementThreshold   usdt.Amount
	SettlementFee         usdt.Amount
	Tick                  time.Duration
	BatchSize             int
	SimulatedConfirmDelay time.Duration
	TronGridURL           string
	TronGridAPIKey        string
	SignerURL             string
	SignerToken           string
	EnergyProvider        string
	TronSaveURL           string
	MaxEnergyPriceSun     float64
	EnergyRentalSeconds   int
}

func LoadSweeper(lookup LookupFunc) (Sweeper, error) {
	reader := newReader(lookup)
	cfg := Sweeper{
		Common:                readCommon(reader),
		Tick:                  reader.duration("GIGJAR_SWEEPER_TICK", 3*time.Second),
		BatchSize:             reader.integer("GIGJAR_SWEEPER_BATCH_SIZE", 20),
		SimulatedConfirmDelay: reader.duration("GIGJAR_SIMULATED_SWEEP_DELAY", 10*time.Second),
	}
	cfg.SettlementThreshold, cfg.SettlementFee = readSettlement(reader)
	reader.requirePositive("GIGJAR_SWEEPER_TICK", cfg.Tick)
	if cfg.BatchSize < 1 || cfg.BatchSize > 500 {
		reader.fail("GIGJAR_SWEEPER_BATCH_SIZE", "must be between 1 and 500")
	}
	if defaultTronGridURL, ok := trongrid.DefaultBaseURL(cfg.Network); ok {
		cfg.TronGridURL = strings.TrimRight(reader.string("GIGJAR_TRONGRID_URL", defaultTronGridURL), "/")
		cfg.TronGridAPIKey = reader.required("GIGJAR_TRONGRID_API_KEY")
		cfg.SignerURL = strings.TrimRight(reader.string("GIGJAR_SIGNER_URL", "http://signer:8081"), "/")
		if !strings.Contains(cfg.SignerURL, "://") {
			cfg.SignerURL = "http://" + cfg.SignerURL
		}
		if parsed, err := url.Parse(cfg.SignerURL); err != nil || parsed.Host == "" {
			reader.fail("GIGJAR_SIGNER_URL", "must be a URL or host:port")
		}
		cfg.SignerToken = reader.required("GIGJAR_SIGNER_TOKEN")
		if len(cfg.SignerToken) < 32 {
			reader.fail("GIGJAR_SIGNER_TOKEN", "must be at least 32 characters")
		}
		cfg.EnergyProvider = strings.ToLower(reader.string("GIGJAR_ENERGY_PROVIDER", "tronsave"))
		switch cfg.EnergyProvider {
		case "tronsave", "none":
		default:
			reader.fail("GIGJAR_ENERGY_PROVIDER", "must be tronsave or none")
		}
		cfg.TronSaveURL = strings.TrimRight(reader.string("GIGJAR_TRONSAVE_URL", ""), "/")
		cfg.MaxEnergyPriceSun = float64(reader.integer("GIGJAR_MAX_ENERGY_PRICE_SUN", 90))
		cfg.EnergyRentalSeconds = reader.integer("GIGJAR_ENERGY_RENTAL_SECONDS", 900)
		if cfg.MaxEnergyPriceSun <= 0 || cfg.MaxEnergyPriceSun >= 100 {
			reader.fail("GIGJAR_MAX_ENERGY_PRICE_SUN", "must be between 1 and 99 (burning costs 100)")
		}
		if cfg.EnergyRentalSeconds < 900 {
			reader.fail("GIGJAR_ENERGY_RENTAL_SECONDS", "must be at least 900")
		}
	}
	return cfg, reader.err()
}

func LoadWatcher(lookup LookupFunc) (Watcher, error) {
	reader := newReader(lookup)
	cfg := Watcher{
		Common:             readCommon(reader),
		Tick:               reader.duration("GIGJAR_WATCHER_TICK", 3*time.Second),
		BatchSize:          reader.integer("GIGJAR_WATCHER_BATCH_SIZE", 25),
		ActivePollInterval: reader.duration("GIGJAR_WATCHER_ACTIVE_POLL_INTERVAL", 15*time.Second),
		IdlePollInterval:   reader.duration("GIGJAR_WATCHER_IDLE_POLL_INTERVAL", 5*time.Minute),
		CursorOverlap:      reader.duration("GIGJAR_WATCHER_CURSOR_OVERLAP", 10*time.Minute),
		DropAfter:          reader.duration("GIGJAR_WATCHER_DROP_AFTER", 24*time.Hour),
		ReconcileInterval:  reader.duration("GIGJAR_WATCHER_RECONCILE_INTERVAL", 10*time.Minute),
	}
	for key, value := range map[string]time.Duration{
		"GIGJAR_WATCHER_TICK":                 cfg.Tick,
		"GIGJAR_WATCHER_ACTIVE_POLL_INTERVAL": cfg.ActivePollInterval,
		"GIGJAR_WATCHER_IDLE_POLL_INTERVAL":   cfg.IdlePollInterval,
		"GIGJAR_WATCHER_CURSOR_OVERLAP":       cfg.CursorOverlap,
		"GIGJAR_WATCHER_DROP_AFTER":           cfg.DropAfter,
		"GIGJAR_WATCHER_RECONCILE_INTERVAL":   cfg.ReconcileInterval,
	} {
		reader.requirePositive(key, value)
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > 500 {
		reader.fail("GIGJAR_WATCHER_BATCH_SIZE", "must be between 1 and 500")
	}
	if defaultTronGridURL, ok := trongrid.DefaultBaseURL(cfg.Network); ok {
		cfg.TronGridURL = strings.TrimRight(reader.string("GIGJAR_TRONGRID_URL", defaultTronGridURL), "/")
		cfg.TronGridAPIKey = reader.required("GIGJAR_TRONGRID_API_KEY")
		if parsed, err := url.Parse(cfg.TronGridURL); err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			reader.fail("GIGJAR_TRONGRID_URL", "must be an absolute https URL")
		}
	}
	return cfg, reader.err()
}

func readSettlement(reader *reader) (usdt.Amount, usdt.Amount) {
	threshold := reader.amount("GIGJAR_SETTLEMENT_THRESHOLD", usdt.MustParse("12"))
	fee := reader.amount("GIGJAR_SETTLEMENT_FEE", usdt.MustParse("1"))
	if fee < 0 {
		reader.fail("GIGJAR_SETTLEMENT_FEE", "must not be negative")
	}
	if threshold-fee <= clevaMinimumDeposit {
		reader.fail("GIGJAR_SETTLEMENT_THRESHOLD", "minus GIGJAR_SETTLEMENT_FEE must stay above Cleva's 10 USDT minimum deposit")
	}
	return threshold, fee
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

func (r *reader) integer(key string, fallback int) int {
	value, ok := r.raw(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		r.fail(key, fmt.Sprintf("invalid integer %q", value))
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
