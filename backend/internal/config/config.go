package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port          string
	DBPath        string
	ImageCacheDir string
	// Forces tournament-window logic to always return fake data regardless of day/time.
	DevMode    bool
	FixtureDir string
	// When true, the weekly index scrape and the today-result API both refuse
	// to make any request to playerzone unless robots.txt explicitly allows
	// RGBlitzBot for every path they need — see internal/scraper/robots.go.
	RespectPermission bool
	// Resend API key/sender/recipient for the feedback email notification.
	// If ResendAPIKey or FeedbackNotifyEmail is empty, notifications silently
	// no-op — feedback is still stored in the database either way.
	ResendAPIKey        string
	ResendFromEmail     string
	FeedbackNotifyEmail string
}

func Load() *Config {
	return &Config{
		Port:                getEnv("PORT", "8080"),
		DBPath:              getEnv("DB_PATH", "./data/players.db"),
		ImageCacheDir:       getEnv("IMAGE_CACHE_DIR", "./cache/images"),
		DevMode:             getBoolEnv("DEV_MODE", false),
		FixtureDir:          getEnv("FIXTURE_DIR", ""),
		RespectPermission:   getBoolEnv("RESPECT_PERMISSION", false),
		ResendAPIKey:        getEnv("RESEND_API_KEY", ""),
		ResendFromEmail:     getEnv("RESEND_FROM_EMAIL", ""),
		FeedbackNotifyEmail: getEnv("FEEDBACK_NOTIFY_EMAIL", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBoolEnv(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
