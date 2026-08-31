package internal

import (
	"os"
	"strconv"
)

// Config holds content-safety plugin runtime configuration.
type Config struct {
	Port        int
	Host        string
	DatabaseURL string
	APIKey      string
}

// LoadConfig reads config from environment variables with defaults.
func LoadConfig() Config {
	port := 3213
	if v := os.Getenv("CONTENT_SAFETY_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	host := os.Getenv("CONTENT_SAFETY_PLUGIN_HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	return Config{
		Port:        port,
		Host:        host,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIKey:      os.Getenv("CONTENT_SAFETY_API_KEY"),
	}
}
