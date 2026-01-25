package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
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
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       0,
		},
		JWT: JWTConfig{
			SecretKey:       getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
			ExpirationHours: 24,
		},
	}

	return config, nil
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
