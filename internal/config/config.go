package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Server config
	Server ServerConfig

	// database config
	Database DatabaseConfig

	// CSRF config
	Security SecurityConfig

	// PPLX API config
	APIs APIConfig

	// feature flags and limits
	Limits LimitsConfig
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port        string
	Environment string // development, staging, production
	BaseURL     string
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	URL string
}

// SecurityConfig holds security-related settings.
type SecurityConfig struct {
	CSRFSecret        string
	SessionCookieName string
	SessionDuration   time.Duration
	BcryptCost        int
	SecureCookies     bool // true in production
}

// APIConfig holds external API configuration.
type APIConfig struct {
	PerplexityAPIKey string
	PerplexityModel  string
	GitHubAPIBaseURL string
}

// LimitsConfig holds rate limiting and quota settings.
type LimitsConfig struct {
	DefaultUserQuota int
	MaxReposPerUser  int
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Server.Environment == "development"
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Server.Environment == "production"
}

func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	// This is useful for local development but not required in production
	// where env vars are typically set by the orchestration platform
	_ = godotenv.Load()

	cfg := &Config{}

	// Load server configuration
	cfg.Server = ServerConfig{
		Port:        getEnvOrDefault("SERVER_PORT", "8080"),
		Environment: getEnvOrDefault("APP_ENV", "development"),
		BaseURL:     getEnvOrDefault("BASE_URL", "http://localhost:8080"),
	}

	// Load database configuration
	cfg.Database = DatabaseConfig{
		URL: os.Getenv("DATABASE_URL"),
	}

	// Load security configuration
	sessionHours, err := strconv.Atoi(getEnvOrDefault("SESSION_DURATION_HOURS", "24"))
	if err != nil {
		return nil, fmt.Errorf("invalid SESSION_DURATION_HOURS: %w", err)
	}

	bcryptCost, err := strconv.Atoi(getEnvOrDefault("BCRYPT_COST", "12"))
	if err != nil {
		return nil, fmt.Errorf("invalid BCRYPT_COST: %w", err)
	}

	cfg.Security = SecurityConfig{
		CSRFSecret:        os.Getenv("CSRF_SECRET"),
		SessionCookieName: getEnvOrDefault("SESSION_COOKIE_NAME", "github_analyzer_session"),
		SessionDuration:   time.Duration(sessionHours) * time.Hour,
		BcryptCost:        bcryptCost,
		SecureCookies:     cfg.Server.Environment == "production",
	}

	// Load API configuration
	cfg.APIs = APIConfig{
		PerplexityAPIKey: os.Getenv("PERPLEXITY_API_KEY"),
		PerplexityModel:  getEnvOrDefault("PERPLEXITY_MODEL", "llama-3.1-sonar-large-128k-online"),
		GitHubAPIBaseURL: getEnvOrDefault("GITHUB_API_BASE_URL", "https://api.github.com"),
	}

	// Load limits configuration
	defaultQuota, err := strconv.Atoi(getEnvOrDefault("DEFAULT_USER_QUOTA", "100000"))
	if err != nil {
		return nil, fmt.Errorf("invalid DEFAULT_USER_QUOTA: %w", err)
	}

	maxRepos, err := strconv.Atoi(getEnvOrDefault("MAX_REPOS_PER_USER", "50"))
	if err != nil {
		return nil, fmt.Errorf("invalid MAX_REPOS_PER_USER: %w", err)
	}

	cfg.Limits = LimitsConfig{
		DefaultUserQuota: defaultQuota,
		MaxReposPerUser:  maxRepos,
	}

	// Validate required configuration
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate checks that all required configuration is present and valid.
// This implements the "fail fast" principle - better to fail at startup
// than to fail later when a missing config is accessed.
func (c *Config) validate() error {
	var errs []error

	// Database URL is always required
	if c.Database.URL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	// CSRF secret must be set and sufficiently long
	if c.Security.CSRFSecret == "" {
		errs = append(errs, errors.New("CSRF_SECRET is required"))
	} else if len(c.Security.CSRFSecret) < 32 {
		errs = append(errs, errors.New("CSRF_SECRET must be at least 32 characters"))
	}

	// Perplexity API key is required for analysis features
	if c.APIs.PerplexityAPIKey == "" {
		errs = append(errs, errors.New("PERPLEXITY_API_KEY is required"))
	}

	// Validate bcrypt cost is in reasonable range
	// Cost < 10 is too fast (vulnerable to brute force)
	// Cost > 16 is too slow (poor user experience)
	if c.Security.BcryptCost < 10 || c.Security.BcryptCost > 16 {
		errs = append(errs, errors.New("BCRYPT_COST must be between 10 and 16"))
	}

	// Validate environment is a known value
	validEnvs := map[string]bool{
		"development": true,
		"staging":     true,
		"production":  true,
	}
	if !validEnvs[c.Server.Environment] {
		errs = append(errs, fmt.Errorf("APP_ENV must be one of: development, staging, production (got: %s)", c.Server.Environment))
	}

	// Combine all errors
	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed:\n%w", errors.Join(errs...))
	}

	return nil
}

// getEnvOrDefault returns the .env value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// MustLoad is like Load but panics on error.
// Used in main() where its required to fail fast
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load configuration: %v", err))
	}
	return cfg
}
