package internal

import "os"

// Config holds runtime configuration for the plugin.
type Config struct {
	Port        string
	DatabaseURL string
}

// LoadConfig reads configuration from environment variables.
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3125"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}

	return Config{
		Port:        port,
		DatabaseURL: dbURL,
	}
}
