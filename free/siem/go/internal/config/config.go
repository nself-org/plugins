// Package config loads SIEM plugin configuration from environment variables.
package config

import (
	"os"
	"strconv"
)

// Config holds all SIEM plugin runtime configuration.
type Config struct {
	DatabaseURL     string
	Port            string
	Enabled         bool   // NSELF_SIEM — enable external SIEM shipping
	HighFreq        bool   // NSELF_SIEM_HF — < 5s flush (Enterprise)
	DefaultSchema   string // ocsf|ecs|raw
	BatchSize       int
	FlushIntervalS  int
	LokiURL         string
	LogLevel        string
}

// Load reads configuration from environment with defaults.
func Load() *Config {
	cfg := &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		Port:           envOrDefault("SIEM_PORT", "8002"),
		Enabled:        envBool("NSELF_SIEM", false),
		HighFreq:       envBool("NSELF_SIEM_HF", false),
		DefaultSchema:  envOrDefault("NSELF_SIEM_DEFAULT_SCHEMA", "ocsf"),
		BatchSize:      envInt("NSELF_SIEM_BATCH_SIZE", 500),
		FlushIntervalS: envInt("NSELF_SIEM_FLUSH_INTERVAL", 30),
		LokiURL:        envOrDefault("NSELF_SIEM_LOKI_URL", "http://loki:3100"),
		LogLevel:       envOrDefault("LOG_LEVEL", "info"),
	}
	return cfg
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
