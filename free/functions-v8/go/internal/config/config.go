// Package config loads and validates environment configuration for functions-v8.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds runtime configuration for the functions-edge plugin.
type Config struct {
	DatabaseURL       string
	Port              int
	MaxDurationMS     int
	PoolSize          int
	MaxMemoryMB       int
	LogRetentionDays  int
	DenoBinaryPath    string
	InternalSecret    string
}

// Load reads configuration from environment variables applying defaults.
func Load() *Config {
	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		Port:             getEnvInt("FUNCTIONS_EDGE_PORT", 3088),
		MaxDurationMS:    getEnvInt("FUNCTIONS_EDGE_MAX_DURATION_MS", 30000),
		PoolSize:         getEnvInt("FUNCTIONS_EDGE_POOL_SIZE", 5),
		MaxMemoryMB:      getEnvInt("FUNCTIONS_EDGE_MAX_MEMORY_MB", 128),
		LogRetentionDays: getEnvInt("FUNCTIONS_EDGE_LOG_RETENTION_DAYS", 7),
		DenoBinaryPath:   getEnvStr("DENO_BINARY_PATH", "/usr/bin/deno"),
		InternalSecret:   os.Getenv("PLUGIN_INTERNAL_SECRET"),
	}
	return cfg
}

// Validate returns an error if required fields are missing.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.PoolSize < 1 {
		return fmt.Errorf("FUNCTIONS_EDGE_POOL_SIZE must be >= 1")
	}
	if c.MaxDurationMS < 1000 {
		return fmt.Errorf("FUNCTIONS_EDGE_MAX_DURATION_MS must be >= 1000")
	}
	return nil
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
