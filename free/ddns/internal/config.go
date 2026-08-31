package internal

import (
	"os"
	"strconv"
)

// Config holds runtime configuration for the ddns plugin.
type Config struct {
	Port        int
	Host        string
	DatabaseURL string
	APIKey      string
}

// DefaultPort is the reserved port for this plugin.
const DefaultPort = 3217

// LoadConfig reads environment variables and returns a Config.
func LoadConfig() Config {
	port := DefaultPort
	if v := os.Getenv("DDNS_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	host := "0.0.0.0"
	if v := os.Getenv("DDNS_HOST"); v != "" {
		host = v
	}

	return Config{
		Port:        port,
		Host:        host,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIKey:      os.Getenv("DDNS_API_KEY"),
	}
}
