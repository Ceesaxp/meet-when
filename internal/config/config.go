package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	OAuth    OAuthConfig
	Email    EmailConfig
	App      AppConfig
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Address string
	BaseURL string
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver         string // "postgres" or "sqlite"
	Host           string
	Port           int
	User           string
	Password       string
	Name           string
	SSLMode        string
	MigrationsPath string
}

// OAuthConfig holds OAuth provider configurations
type OAuthConfig struct {
	Google GoogleOAuthConfig
	Zoom   ZoomOAuthConfig
}

// GoogleOAuthConfig holds Google OAuth configuration
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// ZoomOAuthConfig holds Zoom OAuth configuration
type ZoomOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// EmailConfig holds email configuration
type EmailConfig struct {
	Provider    string // mailgun, smtp
	FromAddress string
	FromName    string

	// Mailgun specific
	MailgunDomain string
	MailgunAPIKey string

	// SMTP specific
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
}

// AppConfig holds application-specific configuration
type AppConfig struct {
	Environment         string
	MaxSchedulingDays   int
	SessionDuration     time.Duration
	DefaultTimezone     string
	EncryptionKey       string
}

// ConnectionString returns the database connection string
func (d DatabaseConfig) ConnectionString() string {
	if d.Driver == "sqlite" {
		return d.Name // For SQLite, Name is the file path
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Address: getEnv("SERVER_ADDRESS", ":8080"),
			BaseURL: getEnv("BASE_URL", "http://localhost:8080"),
		},
		Database: DatabaseConfig{
			Driver:         getEnv("DB_DRIVER", "sqlite"),
			Host:           getEnv("DB_HOST", "localhost"),
			Port:           getEnvInt("DB_PORT", 5432),
			User:           getEnv("DB_USER", "meetwhen"),
			Password:       getEnv("DB_PASSWORD", "meetwhen"),
			Name:           getEnv("DB_NAME", "meetwhen.db"),
			SSLMode:        getEnv("DB_SSLMODE", "disable"),
			MigrationsPath: getEnv("MIGRATIONS_PATH", "migrations"),
		},
		OAuth: OAuthConfig{
			Google: GoogleOAuthConfig{
				ClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
				ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
				RedirectURL:  getEnv("GOOGLE_REDIRECT_URL", ""),
			},
			Zoom: ZoomOAuthConfig{
				ClientID:     getEnv("ZOOM_CLIENT_ID", ""),
				ClientSecret: getEnv("ZOOM_CLIENT_SECRET", ""),
				RedirectURL:  getEnv("ZOOM_REDIRECT_URL", ""),
			},
		},
		Email: EmailConfig{
			Provider:      getEnv("EMAIL_PROVIDER", "smtp"),
			FromAddress:   getEnv("EMAIL_FROM_ADDRESS", "noreply@localhost"),
			FromName:      getEnv("EMAIL_FROM_NAME", "Meet When"),
			MailgunDomain: getEnv("MAILGUN_DOMAIN", ""),
			MailgunAPIKey: getEnv("MAILGUN_API_KEY", ""),
			SMTPHost:      getEnv("SMTP_HOST", "localhost"),
			SMTPPort:      getEnvInt("SMTP_PORT", 587),
			SMTPUser:      getEnv("SMTP_USER", ""),
			SMTPPassword:  getEnv("SMTP_PASSWORD", ""),
		},
		App: AppConfig{
			Environment:       getEnv("APP_ENV", "development"),
			MaxSchedulingDays: getEnvInt("MAX_SCHEDULING_DAYS", 90),
			SessionDuration:   time.Duration(getEnvInt("SESSION_DURATION_HOURS", 24)) * time.Hour,
			DefaultTimezone:   getEnv("DEFAULT_TIMEZONE", "UTC"),
			EncryptionKey:     getEnv("ENCRYPTION_KEY", ""),
		},
	}

	// Validate required configuration
	if cfg.App.EncryptionKey == "" && cfg.App.Environment == "production" {
		return nil, fmt.Errorf("ENCRYPTION_KEY is required in production")
	}

	// Set default encryption key for development
	if cfg.App.EncryptionKey == "" {
		cfg.App.EncryptionKey = "development-key-32-bytes-long!!"
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
