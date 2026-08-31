package internal

import (
	"os"
	"strconv"
)

// Config holds runtime configuration for the analytics plugin.
type Config struct {
	Port        int
	Host        string
	DatabaseURL string
	APIKey      string
	BatchSize   int
}

// DefaultPort is the reserved port for this plugin.
const DefaultPort = 3206

// LoadConfig reads environment variables and returns a Config.
func LoadConfig() Config {
	port := DefaultPort
	if v := os.Getenv("ANALYTICS_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	batchSize := 100
	if v := os.Getenv("ANALYTICS_BATCH_SIZE"); v != "" {
		if b, err := strconv.Atoi(v); err == nil {
			batchSize = b
		}
	}

	host := "0.0.0.0"
	if v := os.Getenv("ANALYTICS_HOST"); v != "" {
		host = v
	}

	return Config{
		Port:        port,
		Host:        host,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		APIKey:      os.Getenv("ANALYTICS_API_KEY"),
		BatchSize:   batchSize,
	}
}
