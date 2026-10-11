package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds runtime settings, all read from environment variables.
type Config struct {
	AppEnv      string
	Port        string
	DatabaseURL string
	JWTSecret   string
	AccessTTL   time.Duration
	RefreshTTL  time.Duration

	// Shop settings.
	ShopName    string
	PromptPayID string   // phone (10 digits) or national/tax id (13 digits)
	AdminEmails []string // accounts registered with these emails become shop admins
}

func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:      getenv("APP_ENV", "development"),
		Port:        getenv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		AccessTTL:   getDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:  getDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		ShopName:    getenv("SHOP_NAME", "ร้านกาแฟตัวอย่าง"),
		PromptPayID: os.Getenv("PROMPTPAY_ID"),
		AdminEmails: splitEmails(os.Getenv("ADMIN_EMAILS")),
		UploadDir:   getenv("UPLOAD_DIR", "/data/uploads"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}
	if cfg.PromptPayID == "" {
		return nil, fmt.Errorf("PROMPTPAY_ID is required (phone or national id that receives payments)")
	}
	return cfg, nil
}

func splitEmails(v string) []string {
	var out []string
	for _, e := range strings.Split(v, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			out = append(out, e)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
