package internal

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration for the e2ee key-directory plugin.
//
// Purpose:   Centralize env-var parsing so handlers receive validated values.
// Inputs:    Process environment (E2EE_* vars, DATABASE_URL).
// Outputs:   *Config consumed by NewPool + NewHandlers.
// Constraints: DATABASE_URL is required; the server NEVER reads or stores any
//   private key material, so there is intentionally no encryption-key env var.
type Config struct {
	Port        string
	Host        string
	DatabaseURL string

	// Upper bounds on how many one-time / Kyber prekeys a single device may
	// publish in one request. Caps a malicious client from exhausting storage.
	MaxOneTimePreKeys int
	MaxKyberPreKeys   int
}

// LoadConfig reads configuration from environment variables, applying defaults.
func LoadConfig() *Config {
	port := os.Getenv("E2EE_PLUGIN_PORT")
	if port == "" {
		port = "3055"
	}
	host := os.Getenv("E2EE_PLUGIN_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	return &Config{
		Port:              port,
		Host:              host,
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		MaxOneTimePreKeys: envInt("E2EE_MAX_ONE_TIME_PREKEYS", 100),
		MaxKyberPreKeys:   envInt("E2EE_MAX_KYBER_PREKEYS", 100),
	}
}

// envInt reads an integer env var, returning def when unset or unparseable.
func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
