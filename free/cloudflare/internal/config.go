package internal

import (
	"os"
	"strconv"
)

// Config holds runtime configuration for the cloudflare plugin.
type Config struct {
	Port        int
	Host        string
	DatabaseURL string
	CFAPIToken  string
	CFAccountID string
}

// DefaultPort is the reserved port for this plugin.
const DefaultPort = 3024

// LoadConfig reads environment variables and returns a Config.
func LoadConfig() Config {
	port := DefaultPort
	if v := os.Getenv("CF_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	host := "0.0.0.0"
	if v := os.Getenv("CF_PLUGIN_HOST"); v != "" {
		host = v
	}

	return Config{
		Port:        port,
		Host:        host,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		CFAPIToken:  os.Getenv("CF_API_TOKEN"),
		CFAccountID: os.Getenv("CF_ACCOUNT_ID"),
	}
}
