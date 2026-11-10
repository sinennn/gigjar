package config

import (
	"strings"
	"testing"
	"time"

	"gigjar/internal/tron"
	"gigjar/internal/usdt"
)

func lookupFrom(values map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestLoadAPIAppliesDefaults(t *testing.T) {
	cfg, err := LoadAPI(lookupFrom(map[string]string{
		"GIGJAR_DATABASE_URL": "postgres://localhost/gigjar",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Environment != EnvironmentDevelopment {
		t.Errorf("environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("http addr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.RequestTimeout != 15*time.Second {
		t.Errorf("request timeout = %v, want 15s", cfg.RequestTimeout)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:3000" {
		t.Errorf("cors origins = %v", cfg.CORSAllowedOrigins)
	}
}

func TestLoadAPIParsesOverrides(t *testing.T) {
	cfg, err := LoadAPI(lookupFrom(map[string]string{
		"GIGJAR_ENV":                  "production",
		"GIGJAR_DATABASE_URL":         "postgres://db/gigjar",
		"GIGJAR_HTTP_ADDR":            ":9000",
		"GIGJAR_LOG_LEVEL":            "DEBUG",
		"GIGJAR_LOG_FORMAT":           "text",
		"GIGJAR_CORS_ALLOWED_ORIGINS": " https://gigjar.app , https://www.gigjar.app ,",
		"GIGJAR_HTTP_REQUEST_TIMEOUT": "30s",
		"GIGJAR_CHAIN_NETWORK":        "tron-mainnet",
		"GIGJAR_SETTLEMENT_THRESHOLD": "12.5",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "debug" || cfg.LogFormat != "text" || cfg.HTTPAddr != ":9000" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if got := strings.Join(cfg.CORSAllowedOrigins, "|"); got != "https://gigjar.app|https://www.gigjar.app" {
		t.Errorf("cors origins = %q", got)
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("request timeout = %v", cfg.RequestTimeout)
	}
}

func TestLoadAPIReportsEveryProblem(t *testing.T) {
	_, err := LoadAPI(lookupFrom(map[string]string{
		"GIGJAR_ENV":                  "staging",
		"GIGJAR_LOG_LEVEL":            "verbose",
		"GIGJAR_HTTP_REQUEST_TIMEOUT": "soon",
		"GIGJAR_SHUTDOWN_TIMEOUT":     "-1s",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{
		"GIGJAR_DATABASE_URL",
		"GIGJAR_ENV",
		"GIGJAR_LOG_LEVEL",
		"GIGJAR_HTTP_REQUEST_TIMEOUT",
		"GIGJAR_SHUTDOWN_TIMEOUT",
	} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

func TestLoadAPIRejectsWildcardCORSInProduction(t *testing.T) {
	_, err := LoadAPI(lookupFrom(map[string]string{
		"GIGJAR_ENV":                  "production",
		"GIGJAR_DATABASE_URL":         "postgres://db/gigjar",
		"GIGJAR_CHAIN_NETWORK":        "tron-mainnet",
		"GIGJAR_CORS_ALLOWED_ORIGINS": "*",
	}))
	if err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("expected wildcard rejection, got %v", err)
	}
}

func TestBlankValuesFallBackToDefaults(t *testing.T) {
	cfg, err := LoadMigrate(lookupFrom(map[string]string{
		"GIGJAR_DATABASE_URL": "postgres://db/gigjar",
		"GIGJAR_LOG_LEVEL":    "   ",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("log level = %q, want info", cfg.LogLevel)
	}
}

func TestLoadAPIFinancialDefaults(t *testing.T) {
	cfg, err := LoadAPI(lookupFrom(map[string]string{"GIGJAR_DATABASE_URL": "postgres://db/gigjar"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Network != tron.NetworkSimulated {
		t.Errorf("network = %s, want simulated in development", cfg.Network)
	}
	if cfg.SettlementThreshold != usdt.MustParse("11") {
		t.Errorf("threshold = %s", cfg.SettlementThreshold)
	}
	if cfg.SecureCookies {
		t.Error("secure cookies should default off outside production")
	}
}

func TestProductionGuards(t *testing.T) {
	_, err := LoadAPI(lookupFrom(map[string]string{
		"GIGJAR_ENV":                  "production",
		"GIGJAR_DATABASE_URL":         "postgres://db/gigjar",
		"GIGJAR_SECURE_COOKIES":       "false",
		"GIGJAR_SETTLEMENT_THRESHOLD": "10",
	}))
	if err == nil {
		t.Fatal("expected production guard errors")
	}
	for _, key := range []string{"GIGJAR_CHAIN_NETWORK", "GIGJAR_SECURE_COOKIES", "GIGJAR_SETTLEMENT_THRESHOLD"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}

func TestLoadWatcherSimulatedNeedsNoTronGrid(t *testing.T) {
	cfg, err := LoadWatcher(lookupFrom(map[string]string{"GIGJAR_DATABASE_URL": "postgres://localhost/gigjar"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Network != tron.NetworkSimulated || cfg.TronGridURL != "" || cfg.TronGridAPIKey != "" {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.ActivePollInterval != 15*time.Second || cfg.BatchSize != 25 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadWatcherRealNetworkRequiresAPIKey(t *testing.T) {
	_, err := LoadWatcher(lookupFrom(map[string]string{
		"GIGJAR_DATABASE_URL":  "postgres://localhost/gigjar",
		"GIGJAR_CHAIN_NETWORK": "tron-nile",
	}))
	if err == nil || !strings.Contains(err.Error(), "GIGJAR_TRONGRID_API_KEY") {
		t.Fatalf("err = %v", err)
	}

	cfg, err := LoadWatcher(lookupFrom(map[string]string{
		"GIGJAR_DATABASE_URL":     "postgres://localhost/gigjar",
		"GIGJAR_CHAIN_NETWORK":    "tron-nile",
		"GIGJAR_TRONGRID_API_KEY": "key",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TronGridURL != "https://nile.trongrid.io" {
		t.Errorf("trongrid url = %q", cfg.TronGridURL)
	}
}

func TestLoadWatcherRejectsBadValues(t *testing.T) {
	_, err := LoadWatcher(lookupFrom(map[string]string{
		"GIGJAR_DATABASE_URL":                 "postgres://localhost/gigjar",
		"GIGJAR_CHAIN_NETWORK":                "tron-mainnet",
		"GIGJAR_TRONGRID_API_KEY":             "key",
		"GIGJAR_TRONGRID_URL":                 "http://api.trongrid.io",
		"GIGJAR_WATCHER_BATCH_SIZE":           "0",
		"GIGJAR_WATCHER_ACTIVE_POLL_INTERVAL": "-1s",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"GIGJAR_TRONGRID_URL", "GIGJAR_WATCHER_BATCH_SIZE", "GIGJAR_WATCHER_ACTIVE_POLL_INTERVAL"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error does not mention %s: %v", key, err)
		}
	}
}
