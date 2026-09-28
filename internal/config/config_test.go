package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Postgres.Host != defaultPostgresHost {
		t.Errorf("Postgres.Host = %q, want %q", cfg.Postgres.Host, defaultPostgresHost)
	}
	if cfg.Postgres.Port != defaultPostgresPort {
		t.Errorf("Postgres.Port = %d, want %d", cfg.Postgres.Port, defaultPostgresPort)
	}
	if cfg.JWT.AccessTTL != defaultJWTAccessTTL {
		t.Errorf("JWT.AccessTTL = %v, want %v", cfg.JWT.AccessTTL, defaultJWTAccessTTL)
	}
	if cfg.LLM.Provider != defaultLLMProvider {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, defaultLLMProvider)
	}
	if !cfg.UsesMockLLM() {
		t.Error("UsesMockLLM() = false, want true with default provider")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvPostgresHost, "db.internal")
	t.Setenv(EnvPostgresPort, "6543")
	t.Setenv(EnvJWTAccessTTL, "1m")
	t.Setenv(EnvJWTSecret, "0123456789abcdef0123456789abcdef")
	t.Setenv(EnvLLMProvider, "openai")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Postgres.Host != "db.internal" {
		t.Errorf("Postgres.Host = %q, want %q", cfg.Postgres.Host, "db.internal")
	}
	if cfg.Postgres.Port != 6543 {
		t.Errorf("Postgres.Port = %d, want 6543", cfg.Postgres.Port)
	}
	if cfg.JWT.AccessTTL != time.Minute {
		t.Errorf("JWT.AccessTTL = %v, want 1m", cfg.JWT.AccessTTL)
	}
	if cfg.UsesMockLLM() {
		t.Error("UsesMockLLM() = true, want false with provider=openai")
	}
	if got := cfg.Postgres.DSN(); got == "" {
		t.Error("DSN() must not be empty")
	}
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvJWTSecret, "too-short")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want error for short JWT_SECRET")
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvPostgresPort, "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want error for invalid POSTGRES_PORT")
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvJWTAccessTTL, "banana")

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error, want error for invalid JWT_ACCESS_TTL")
	}
}

// clearEnv, testlerin ortamdan bağımsız olması için ilgili değişkenleri
// boşaltır.
func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		EnvAppEnv, EnvLogLevel, EnvAgentAdvertiseHost,
		EnvPostgresHost, EnvPostgresPort, EnvPostgresUser, EnvPostgresPassword, EnvPostgresDB, EnvPostgresSSLMode,
		EnvJWTSecret, EnvJWTIssuer, EnvJWTAccessTTL, EnvJWTRefreshTTL,
		EnvLLMProvider, EnvLLMOpenAIKey, EnvLLMAnthropicKey, EnvLLMGeminiKey, EnvLLMCompatBaseURL, EnvLLMCompatAPIKey,
		EnvMCPWebSearchKey, EnvMCPSQLiteDir, EnvMCPWebSearchURL,
		EnvBlobStore, EnvBlobFSRoot, EnvMinioEndpoint, EnvMinioAccessKey, EnvMinioSecretKey, EnvMinioBucket,
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}
