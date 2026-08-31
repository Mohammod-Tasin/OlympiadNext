package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env               string
	Port              string
	DatabaseURL       string
	JWTAccessSecret   string
	JWTRefreshSecret  string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	GoogleClientID    string
	CookieDomain      string
	CookieSecure      bool
	CookieSameSite    string
	AllowedOrigins    []string
	BulkSMSBDAPIKey   string
	BulkSMSBDSenderID string
	SMTPHost          string
	SMTPPort          string
	SMTPUsername      string
	SMTPPassword      string
}

// Load reads configuration from the environment and fails fast if a
// required secret is missing, rather than letting the server start
// with an insecure default.
func Load() (*Config, error) {
	cfg := &Config{
		Env:              getEnv("APP_ENV", "development"),
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		GoogleClientID:   os.Getenv("GOOGLE_CLIENT_ID"),
		CookieDomain:     os.Getenv("COOKIE_DOMAIN"),
		CookieSecure:     getEnv("COOKIE_SECURE", "true") == "true",
		// "lax" suits a frontend/backend deployed under the same registrable
		// domain (e.g. app.example.com + api.example.com). Cross-domain
		// deployments (e.g. Vercel + Fly.io on unrelated domains) must set
		// this to "none", which additionally requires COOKIE_SECURE=true.
		CookieSameSite: getEnv("COOKIE_SAME_SITE", "lax"),
		// Two frontends call this API: the client app (default :3000) and
		// the admin console (default :3001). Production overrides this with
		// the real deployed origins.
		AllowedOrigins: parseOrigins(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001")),
		// Optional: when unset, the SMS client falls back to logging OTPs to
		// the console instead of calling out to BulkSMSBD, so local dev works
		// without live SMS credentials.
		BulkSMSBDAPIKey:   os.Getenv("BULKSMSBD_API_KEY"),
		BulkSMSBDSenderID: os.Getenv("BULKSMSBD_SENDER_ID"),
		// Optional: when SMTPUsername/SMTPPassword are unset, the email
		// client falls back to logging OTPs to the console instead of
		// calling out to Gmail, so local dev works without SMTP credentials.
		SMTPHost:     getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:     getEnv("SMTP_PORT", "587"),
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
	}

	var err error
	if cfg.AccessTokenTTL, err = parseDurationEnv("ACCESS_TOKEN_TTL", 15*time.Minute); err != nil {
		return nil, err
	}
	if cfg.RefreshTokenTTL, err = parseDurationEnv("REFRESH_TOKEN_TTL", 30*24*time.Hour); err != nil {
		return nil, err
	}

	required := map[string]string{
		"DATABASE_URL":       cfg.DatabaseURL,
		"JWT_ACCESS_SECRET":  cfg.JWTAccessSecret,
		"JWT_REFRESH_SECRET": cfg.JWTRefreshSecret,
		"GOOGLE_CLIENT_ID":   cfg.GoogleClientID,
	}
	for name, val := range required {
		if val == "" {
			return nil, fmt.Errorf("config: required environment variable %s is not set", name)
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseOrigins splits a comma-separated ALLOWED_ORIGINS value (e.g.
// "https://app.example.com,https://staging.example.com") into a clean
// slice, trimming whitespace and dropping empty entries.
func parseOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: invalid duration for %s: %w", key, err)
	}
	return d, nil
}
