// Package config reads the project settings this plugin needs.
//
// Purpose: the queue command came out of the nSelf CLI, where it read a
// 6,500-line Config struct. It needs the Postgres connection settings. This is
// those, read from the environment.
//
// Inputs: environment variables, which the CLI resolves from the project's
// .env cascade and passes to this process for every variable declared in the
// plugin's manifest.
//
// Outputs: a Config with the field names the moved code already used.
//
// Constraints: this deliberately does NOT read .env files itself. The cascade
// order is defined once, in the CLI (CLI-R18); a second implementation here is
// the drift that rule exists to prevent.
package config

import (
	"os"
	"strconv"
)

// Config is the subset of project configuration this command reads.
type Config struct {
	Postgres PostgresConfig
}

// PostgresConfig holds the database connection settings.
type PostgresConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
}

// Load reads the configuration from the environment. Defaults match the CLI's,
// so an absent value behaves as it did in-tree rather than as an empty string.
func Load(string) (*Config, error) {
	return &Config{Postgres: PostgresConfig{
		Host:     envOr("POSTGRES_HOST", "postgres"),
		Port:     envInt("POSTGRES_PORT", 5432),
		DB:       envOr("POSTGRES_DB", "nself"),
		User:     envOr("POSTGRES_USER", "postgres"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
	}}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
