// Package config, uygulama yapılandırmasını ortam değişkenlerinden yükler ve
// doğrular. Gerçek sırlar koda gömülmez; .env veya secret manager üzerinden
// sağlanır.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Ortam değişkeni anahtarları. Magic string kullanımını önlemek için burada
// sabitlenir.
const (
	EnvAppEnv   = "APP_ENV"
	EnvLogLevel = "LOG_LEVEL"
	// EnvAgentAdvertiseHost, AgentCard'ta ilan edilecek ana bilgisayar adıdır.
	EnvAgentAdvertiseHost = "AGENT_ADVERTISE_HOST"

	EnvPostgresHost     = "POSTGRES_HOST"
	EnvPostgresPort     = "POSTGRES_PORT"
	EnvPostgresUser     = "POSTGRES_USER"
	EnvPostgresPassword = "POSTGRES_PASSWORD"
	EnvPostgresDB       = "POSTGRES_DB"
	EnvPostgresSSLMode  = "POSTGRES_SSLMODE"

	EnvJWTSecret     = "JWT_SECRET"
	EnvJWTIssuer     = "JWT_ISSUER"
	EnvJWTAccessTTL  = "JWT_ACCESS_TTL"
	EnvJWTRefreshTTL = "JWT_REFRESH_TTL"

	EnvLLMProvider      = "LLM_PROVIDER"
	EnvLLMOpenAIKey     = "LLM_OPENAI_API_KEY"
	EnvLLMAnthropicKey  = "LLM_ANTHROPIC_API_KEY"
	EnvLLMGeminiKey     = "LLM_GEMINI_API_KEY"
	EnvLLMCompatBaseURL = "LLM_OPENAI_COMPAT_BASE_URL"
	EnvLLMCompatAPIKey  = "LLM_OPENAI_COMPAT_API_KEY"

	EnvMCPWebSearchKey = "MCP_WEBSEARCH_API_KEY"
	EnvMCPWebSearchURL = "MCP_WEBSEARCH_URL"
	EnvMCPSQLiteDir    = "MCP_SQLITE_DIR"

	EnvBlobStore      = "BLOB_STORE"
	EnvBlobFSRoot     = "BLOB_FS_ROOT"
	EnvMinioEndpoint  = "MINIO_ENDPOINT"
	EnvMinioAccessKey = "MINIO_ACCESS_KEY"
	EnvMinioSecretKey = "MINIO_SECRET_KEY"
	EnvMinioBucket    = "MINIO_BUCKET"
)

// Varsayılan değerler.
const (
	defaultAppEnv        = "development"
	defaultLogLevel      = "info"
	defaultPostgresHost  = "localhost"
	defaultPostgresPort  = 5432
	defaultPostgresUser  = "crew"
	defaultPostgresDB    = "crew"
	defaultPostgresSSL   = "disable"
	defaultJWTIssuer     = "a2a-research-crew"
	defaultJWTAccessTTL  = 15 * time.Minute
	defaultJWTRefreshTTL = 7 * 24 * time.Hour
	defaultLLMProvider   = "mock"
	defaultMCPSQLiteDir  = "./data/sqlite"
	defaultBlobStore     = "fs"
	defaultBlobFSRoot    = "./data/artifacts"
	defaultMinioBucket   = "artifacts"
	defaultAdvertiseHost = "127.0.0.1"
)

// Config, uygulamanın tüm yapılandırmasını taşır.
type Config struct {
	AppEnv   string
	LogLevel string

	// AdvertiseHost, AgentCard içinde duyurulacak ana bilgisayar adıdır
	// (yerel geliştirmede 127.0.0.1, compose'da servis adı).
	AdvertiseHost string

	Postgres PostgresConfig
	JWT      JWTConfig
	LLM      LLMConfig
	MCP      MCPConfig
	Blob     BlobConfig
}

// PostgresConfig, PostgreSQL bağlantı ayarlarını taşır.
type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

// DSN, pgx için bağlantı dizesini üretir.
func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.Database, p.SSLMode,
	)
}

// JWTConfig, JWT imzalama ve doğrulama ayarlarını taşır.
type JWTConfig struct {
	Secret     []byte
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// LLMConfig, LLM sağlayıcı seçimini ve kimlik bilgilerini taşır.
type LLMConfig struct {
	Provider string

	OpenAIAPIKey    string
	AnthropicAPIKey string
	GeminiAPIKey    string

	CompatBaseURL string
	CompatAPIKey  string
}

// MCPConfig, MCP sunucuları için dış araç yapılandırmasını taşır.
type MCPConfig struct {
	WebSearchAPIKey string
	WebSearchURL    string
	SQLiteDir       string
}

// BlobConfig, artefakt blob deposu ayarlarını taşır.
type BlobConfig struct {
	Store string

	FSRoot string

	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioBucket    string
}

// Load, ortam değişkenlerinden yapılandırmayı okur, varsayılanları uygular ve
// doğrular.
func Load() (*Config, error) {
	port, err := getenvInt(EnvPostgresPort, defaultPostgresPort)
	if err != nil {
		return nil, err
	}
	accessTTL, err := getenvDuration(EnvJWTAccessTTL, defaultJWTAccessTTL)
	if err != nil {
		return nil, err
	}
	refreshTTL, err := getenvDuration(EnvJWTRefreshTTL, defaultJWTRefreshTTL)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		AppEnv:        getenv(EnvAppEnv, defaultAppEnv),
		LogLevel:      getenv(EnvLogLevel, defaultLogLevel),
		AdvertiseHost: getenv(EnvAgentAdvertiseHost, defaultAdvertiseHost),
		Postgres: PostgresConfig{
			Host:     getenv(EnvPostgresHost, defaultPostgresHost),
			Port:     port,
			User:     getenv(EnvPostgresUser, defaultPostgresUser),
			Password: os.Getenv(EnvPostgresPassword),
			Database: getenv(EnvPostgresDB, defaultPostgresDB),
			SSLMode:  getenv(EnvPostgresSSLMode, defaultPostgresSSL),
		},
		JWT: JWTConfig{
			Secret:     []byte(os.Getenv(EnvJWTSecret)),
			Issuer:     getenv(EnvJWTIssuer, defaultJWTIssuer),
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		LLM: LLMConfig{
			Provider:        getenv(EnvLLMProvider, defaultLLMProvider),
			OpenAIAPIKey:    os.Getenv(EnvLLMOpenAIKey),
			AnthropicAPIKey: os.Getenv(EnvLLMAnthropicKey),
			GeminiAPIKey:    os.Getenv(EnvLLMGeminiKey),
			CompatBaseURL:   os.Getenv(EnvLLMCompatBaseURL),
			CompatAPIKey:    os.Getenv(EnvLLMCompatAPIKey),
		},
		MCP: MCPConfig{
			WebSearchAPIKey: os.Getenv(EnvMCPWebSearchKey),
			WebSearchURL:    os.Getenv(EnvMCPWebSearchURL),
			SQLiteDir:       getenv(EnvMCPSQLiteDir, defaultMCPSQLiteDir),
		},
		Blob: BlobConfig{
			Store:          getenv(EnvBlobStore, defaultBlobStore),
			FSRoot:         getenv(EnvBlobFSRoot, defaultBlobFSRoot),
			MinioEndpoint:  os.Getenv(EnvMinioEndpoint),
			MinioAccessKey: os.Getenv(EnvMinioAccessKey),
			MinioSecretKey: os.Getenv(EnvMinioSecretKey),
			MinioBucket:    getenv(EnvMinioBucket, defaultMinioBucket),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate, yapılandırmanın tutarlılığını kontrol eder.
func (c *Config) Validate() error {
	var problems []string

	if c.LogLevel == "" {
		problems = append(problems, "LOG_LEVEL must not be empty")
	}
	// JWT yalnızca mock/dry-run dışı üretim akışlarında zorunludur; ancak
	// güvenli varsayılan için secret'ın varlığını ve uzunluğunu her zaman
	// denetleriz.
	if n := len(c.JWT.Secret); n > 0 && n < 32 {
		problems = append(problems, "JWT_SECRET must be at least 32 bytes")
	}
	if c.JWT.AccessTTL <= 0 {
		problems = append(problems, "JWT_ACCESS_TTL must be positive")
	}
	if c.JWT.RefreshTTL <= 0 {
		problems = append(problems, "JWT_REFRESH_TTL must be positive")
	}
	if c.Postgres.Port <= 0 || c.Postgres.Port > 65535 {
		problems = append(problems, "POSTGRES_PORT must be a valid port")
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}
	return nil
}

// UsesMockLLM, gerçek bir LLM anahtarı olmadan çalışıldığını bildirir.
func (c *Config) UsesMockLLM() bool {
	return c.LLM.Provider == "" || c.LLM.Provider == "mock"
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s must be an integer: %w", key, err)
	}
	return v, nil
}

func getenvDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("environment variable %s must be a duration: %w", key, err)
	}
	return v, nil
}
