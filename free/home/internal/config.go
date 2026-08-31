package internal

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL         string
	Port                string
	HAUrl               string
	HAToken             string
	PluginInternalSecret string
}

// validateHAUrl checks that the Home Assistant URL is safe to use:
// - Must be a valid URL with http or https scheme
// - Must not target loopback/localhost unless explicitly configured via HOME_ASSISTANT_ALLOW_LOCAL=1
// - Must have a non-empty host
func validateHAUrl(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("HOME_ASSISTANT_URL is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("HOME_ASSISTANT_URL must use http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("HOME_ASSISTANT_URL must have a non-empty host")
	}
	// Block loopback/localhost unless the operator explicitly opts in
	host := strings.ToLower(u.Hostname())
	isLocal := host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".local")
	if isLocal && os.Getenv("HOME_ASSISTANT_ALLOW_LOCAL") != "1" {
		return fmt.Errorf("HOME_ASSISTANT_URL targets a loopback/local address (%s); set HOME_ASSISTANT_ALLOW_LOCAL=1 to permit", host)
	}
	return nil
}

func LoadConfig() Config {
	port := os.Getenv("HOME_PORT")
	if port == "" {
		port = "3127"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}
	haURL := os.Getenv("HOME_ASSISTANT_URL")
	if haURL == "" {
		haURL = "http://homeassistant.local:8123"
	}
	if err := validateHAUrl(haURL); err != nil {
		// Log and clear the URL so the server starts but HA integration is disabled
		fmt.Fprintf(os.Stderr, "[home-plugin] WARNING: invalid HOME_ASSISTANT_URL — HA integration disabled: %v\n", err)
		haURL = ""
	}
	return Config{
		DatabaseURL:          dbURL,
		Port:                 port,
		HAUrl:                haURL,
		HAToken:              os.Getenv("HOME_ASSISTANT_TOKEN"),
		PluginInternalSecret: os.Getenv("PLUGIN_INTERNAL_SECRET"),
	}
}
