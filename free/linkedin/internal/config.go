package internal

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds runtime configuration for nself-linkedin.
type Config struct {
	DatabaseURL      string
	Port             int
	ClientID         string
	ClientSecret     string
	RedirectURI      string
	InternalSecret   string
}

// LoadConfig reads configuration from environment variables.
// PORT defaults to 3722.
func LoadConfig() (*Config, error) {
	port := 3722
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	cfg := &Config{
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		Port:           port,
		ClientID:       os.Getenv("LINKEDIN_CLIENT_ID"),
		ClientSecret:   os.Getenv("LINKEDIN_CLIENT_SECRET"),
		RedirectURI:    os.Getenv("LINKEDIN_REDIRECT_URI"),
		InternalSecret: os.Getenv("LINKEDIN_INTERNAL_SECRET"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("LINKEDIN_CLIENT_ID is required")
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("LINKEDIN_CLIENT_SECRET is required")
	}
	if cfg.RedirectURI == "" {
		return nil, fmt.Errorf("LINKEDIN_REDIRECT_URI is required")
	}
	if cfg.InternalSecret == "" {
		return nil, fmt.Errorf("LINKEDIN_INTERNAL_SECRET is required")
	}

	return cfg, nil
}
