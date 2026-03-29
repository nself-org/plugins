package sdk

import (
	"os"
	"strconv"
)

// Config holds standard environment configuration for an nSelf plugin.
type Config struct {
	DatabaseURL string
	Port        int
	Secret      string // PLUGIN_INTERNAL_SECRET
}

// LoadConfig reads DATABASE_URL, PORT (default 3000), and
// PLUGIN_INTERNAL_SECRET from the environment.
func LoadConfig() *Config {
	port := 3000
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	return &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        port,
		Secret:      os.Getenv("PLUGIN_INTERNAL_SECRET"),
	}
}
