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
	Bots            BotsConfig
	RateLimits      RateLimitsConfig
}

// RateLimitsConfig holds the per-bucket request ceilings. Each is "how many
// requests per how many seconds"; setting a limit to 0 disables that bucket.
//
// The player-facing buckets are keyed on the authenticated user id rather than
// the IP (see middleware.RateLimit), so they can be tight without punishing the
// many subscribers an Ethiopian mobile carrier puts behind one NAT address.
// The auth buckets have no user to key on yet and are necessarily per-IP, so
// they are deliberately looser than a brute-force limit would otherwise be —
// tight enough to stop credential stuffing, loose enough that a shared carrier
// address is not locked out by one bad actor.
type RateLimitsConfig struct {
	LoginLimit         int // per-IP: admin password login
	LoginWindow        int
	CreateAdminLimit   int // per-IP: secret-code gated admin creation
	CreateAdminWindow  int
	TelegramAuthLimit  int // per-IP: Mini App initData -> JWT
	TelegramAuthWindow int
	DepositLimit       int // per-user: deposit submission/verification
	DepositWindow      int
	WithdrawLimit      int // per-user: withdrawal requests
	WithdrawWindow     int
	TransferLimit      int // per-user: player-to-player transfers
	TransferWindow     int
	WebSocketLimit     int // per-IP: socket connection attempts
	WebSocketWindow    int
}

// BotsConfig holds the runtime knobs for the filler-bot subsystem. The actual
// fill POLICY (enabled, thresholds, target) lives in the DB and is edited from
// the admin dashboard; these are operational defaults set once at deploy time.
type BotsConfig struct {
	Enabled         bool    // run the background auto-filler goroutine at all
	PoolSize        int     // how many bot accounts to seed on boot
	WalletFloat     float64 // house money each bot wallet is topped up to (birr)
	MaxJoinsPerTick int     // bots added per game per sweep (spaces out joins)
	CheckInterval   int     // seconds between auto-fill sweeps
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
	// TelebirrAccount, CBEBirrAccount and MpesaAccount are the house numbers that
	// deposits of each method must be paid to. When set, the verifier rejects any
	// receipt credited to a different account, so a valid receipt for money sent
	// elsewhere can't be claimed. While a method's number is blank its deposits
	// are never auto-credited — they queue for manual admin approval. CBE Birr
	// additionally needs its number to even look receipts up (receipts are
	// fetched by receiver phone).
	TelebirrAccount string
	CBEBirrAccount  string
	MpesaAccount    string
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
			CBEBirrAccount:  getEnv("VERIFY_CBEBIRR_ACCOUNT", ""),
			MpesaAccount:    getEnv("VERIFY_MPESA_ACCOUNT", ""),
		},
		Internal: InternalConfig{
			APISecret: getEnv("INTERNAL_API_SECRET", ""),
		},
		Bots: BotsConfig{
			Enabled:         getEnv("BOTS_ENABLED", "false") == "true",
			PoolSize:        getEnvInt("BOT_POOL_SIZE", 30),
			WalletFloat:     float64(getEnvInt("BOT_WALLET_FLOAT", 1000)),
			MaxJoinsPerTick: getEnvInt("BOT_MAX_JOINS_PER_TICK", 5),
			CheckInterval:   getEnvInt("BOT_CHECK_INTERVAL_SECONDS", 5),
		},
		RateLimits: RateLimitsConfig{
			// Per-IP. 10 attempts per 15 min stops credential stuffing while
			// leaving room for an admin fumbling a password behind shared NAT.
			LoginLimit:  getEnvInt("RL_LOGIN_LIMIT", 10),
			LoginWindow: getEnvInt("RL_LOGIN_WINDOW_SECONDS", 900),
			// Per-IP. Creating an admin is the highest-value action here and
			// legitimately happens a handful of times ever.
			CreateAdminLimit:  getEnvInt("RL_CREATE_ADMIN_LIMIT", 5),
			CreateAdminWindow: getEnvInt("RL_CREATE_ADMIN_WINDOW_SECONDS", 3600),
			// Per-IP, and the one bucket a whole carrier shares — every Mini
			// App open hits it, so it is generous on purpose. It exists to
			// blunt initData forgery attempts, not to pace normal use.
			TelegramAuthLimit:  getEnvInt("RL_TELEGRAM_AUTH_LIMIT", 120),
			TelegramAuthWindow: getEnvInt("RL_TELEGRAM_AUTH_WINDOW_SECONDS", 60),
			// Per-user from here down, so these are about one account's
			// behaviour and NAT is irrelevant.
			DepositLimit:    getEnvInt("RL_DEPOSIT_LIMIT", 10),
			DepositWindow:   getEnvInt("RL_DEPOSIT_WINDOW_SECONDS", 60),
			WithdrawLimit:   getEnvInt("RL_WITHDRAW_LIMIT", 5),
			WithdrawWindow:  getEnvInt("RL_WITHDRAW_WINDOW_SECONDS", 60),
			TransferLimit:   getEnvInt("RL_TRANSFER_LIMIT", 10),
			TransferWindow:  getEnvInt("RL_TRANSFER_WINDOW_SECONDS", 60),
			WebSocketLimit:  getEnvInt("RL_WEBSOCKET_LIMIT", 60),
			WebSocketWindow: getEnvInt("RL_WEBSOCKET_WINDOW_SECONDS", 60),
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
