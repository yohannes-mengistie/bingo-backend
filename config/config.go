package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Server          ServerConfig
	Database        DatabaseConfig
	Redis           RedisConfig
	JWT             JWTConfig
	Admin           AdminConfig
	Telegram        TelegramConfig
	PaymentVerifier PaymentVerifierConfig
	Internal        InternalConfig
}

// InternalConfig gates the server-to-server ("bot-facing") /user, /wallet and
// per-user /games reads. Callers must present APISecret in the
// X-Internal-Api-Secret header. When empty, those endpoints are disabled
// (fail closed) so they can never be reached anonymously from the public net.
type InternalConfig struct {
	APISecret string
}

type ServerConfig struct {
	Port         string
	Host         string
	ReadTimeout  int
	WriteTimeout int
	IdleTimeout  int
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

type JWTConfig struct {
	SecretKey       string
	ExpirationHours int
}

type AdminConfig struct {
	SecretCode string
}

type TelegramConfig struct {
	BotToken      string // used to verify Mini App initData AND authenticate Bot API calls
	WebhookSecret string // shared secret Telegram echoes back in X-Telegram-Bot-Api-Secret-Token
	MiniAppURL    string // URL the bot's "Play" button opens (the Vercel Mini App)
}

type PaymentVerifierConfig struct {
	BaseURL string
	APIKey  string
	// TelebirrAccount is the house Telebirr number that deposits must be paid
	// to. When set, the verifier rejects any receipt credited to a different
	// account, so a valid receipt for money sent elsewhere can't be claimed.
	TelebirrAccount string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load .env file if it exists (optional)
	_ = godotenv.Load()

	config := &Config{
		Server: ServerConfig{
			// Railway uses PORT, fallback to SERVER_PORT
			Port:         getEnv("PORT", getEnv("SERVER_PORT", "8080")),
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			ReadTimeout:  15,
			WriteTimeout: 15,
			IdleTimeout:  60,
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", getEnv("PGHOST", "localhost")),
			Port:     getEnv("DB_PORT", getEnv("PGPORT", "5432")),
			User:     getEnv("DB_USER", getEnv("PGUSER", "postgres")),
			Password: getEnv("DB_PASSWORD", getEnv("PGPASSWORD", "postgres")),
			DBName:   getEnv("DB_NAME", getEnv("PGDATABASE", "bingo")),
			SSLMode:  getEnv("DB_SSLMODE", getEnv("PGSSLMODE", "disable")),
		},
		Redis: parseRedisConfig(),
		JWT: JWTConfig{
			SecretKey:       getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			ExpirationHours: getEnvInt("JWT_EXPIRATION_HOURS", 24),
		},
		Admin: AdminConfig{
			SecretCode: getEnv("SECRET_CODE", ""),
		},
		Telegram: TelegramConfig{
			BotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
			WebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
			MiniAppURL:    getEnv("TELEGRAM_MINIAPP_URL", "https://bingo-miniapp-gold.vercel.app"),
		},
		PaymentVerifier: PaymentVerifierConfig{
			BaseURL:         strings.TrimRight(getEnv("VERIFY_API_BASE_URL", "https://verifyapi.leulzenebe.pro"), "/"),
			APIKey:          getEnv("VERIFY_API_KEY", ""),
			TelebirrAccount: getEnv("VERIFY_TELEBIRR_ACCOUNT", ""),
		},
		Internal: InternalConfig{
			APISecret: getEnv("INTERNAL_API_SECRET", ""),
		},
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// validateConfig fails startup (rather than silently running with a known-weak
// secret) when a security-critical value is missing or left at its placeholder.
func validateConfig(c *Config) error {
	s := c.JWT.SecretKey
	switch {
	case s == "", s == "your-secret-key-change-in-production", s == "change-me":
		return fmt.Errorf("JWT_SECRET is unset or still the placeholder; set a strong random value (openssl rand -hex 32)")
	case len(s) < 32:
		return fmt.Errorf("JWT_SECRET is too short (%d chars); use at least 32 (openssl rand -hex 32)", len(s))
	}
	return nil
}

// GetDSN returns the PostgreSQL connection string
func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

// GetAddr returns the Redis address
func (c *RedisConfig) GetAddr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// parseRedisConfig parses Redis configuration from REDIS_URL or individual environment variables
func parseRedisConfig() RedisConfig {
	redisURL := getEnv("REDIS_URL", "")

	// If REDIS_URL is provided, parse it
	if redisURL != "" {
		parsedURL, err := url.Parse(redisURL)
		if err == nil {
			config := RedisConfig{
				Host:     parsedURL.Hostname(),
				Port:     parsedURL.Port(),
				Password: "",
				DB:       0,
			}

			// Get password from UserInfo
			if parsedURL.User != nil {
				password, ok := parsedURL.User.Password()
				if ok {
					config.Password = password
				}
			}

			// Get database number from path (e.g., /0, /1, etc.)
			if parsedURL.Path != "" {
				dbStr := strings.TrimPrefix(parsedURL.Path, "/")
				if dbNum, err := strconv.Atoi(dbStr); err == nil {
					config.DB = dbNum
				}
			}

			// Default port if not specified
			if config.Port == "" {
				config.Port = "6379"
			}

			return config
		}
	}

	// Fall back to individual environment variables
	return RedisConfig{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	}
}
