package internal

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration for the content-progress plugin.
type Config struct {
	Port                 int
	CompleteThreshold    int
	HistorySampleSeconds int
	HistoryRetentionDays int
}

// LoadConfig reads environment variables and returns a validated Config.
func LoadConfig() Config {
	cfg := Config{
		Port:                 3022,
		CompleteThreshold:    95,
		HistorySampleSeconds: 30,
		HistoryRetentionDays: 365,
	}

	if v := os.Getenv("PROGRESS_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	} else if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}

	if v := os.Getenv("PROGRESS_COMPLETE_THRESHOLD"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			cfg.CompleteThreshold = t
		}
	}

	if v := os.Getenv("PROGRESS_HISTORY_SAMPLE_SECONDS"); v != "" {
		if s, err := strconv.Atoi(v); err == nil {
			cfg.HistorySampleSeconds = s
		}
	}

	if v := os.Getenv("PROGRESS_HISTORY_RETENTION_DAYS"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			cfg.HistoryRetentionDays = d
		}
	}

	return cfg
}

// Validate checks that config values are within acceptable ranges.
func (c Config) Validate() error {
	if c.CompleteThreshold < 1 || c.CompleteThreshold > 100 {
		return fmt.Errorf("PROGRESS_COMPLETE_THRESHOLD must be between 1 and 100")
	}
	if c.HistorySampleSeconds < 1 {
		return fmt.Errorf("PROGRESS_HISTORY_SAMPLE_SECONDS must be at least 1")
	}
	if c.HistoryRetentionDays < 1 {
		return fmt.Errorf("PROGRESS_HISTORY_RETENTION_DAYS must be at least 1")
	}
	return nil
}
