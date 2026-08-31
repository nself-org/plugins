package internal

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration for the compliance plugin.
type Config struct {
	Port        string
	Host        string
	DatabaseURL string
	APIKey      string
	LogLevel    string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	port := os.Getenv("COMPLIANCE_PLUGIN_PORT")
	if port == "" {
		port = strconv.Itoa(DefaultPort)
	}

	host := os.Getenv("COMPLIANCE_PLUGIN_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	return Config{
		Port:        port,
		Host:        host,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIKey:      os.Getenv("COMPLIANCE_API_KEY"),
		LogLevel:    os.Getenv("COMPLIANCE_LOG_LEVEL"),
	}
}

// DefaultPort is the well-known port for this plugin.
const DefaultPort = 3211
