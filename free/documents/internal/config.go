package internal

import "os"

// Config holds runtime configuration for the documents plugin.
type Config struct {
	DatabaseURL string
	Port        string
}

// LoadConfig reads environment variables and returns a Config.
// PORT defaults to 3106. DATABASE_URL falls back to a local dev URL.
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3106"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}
	return Config{DatabaseURL: dbURL, Port: port}
}
