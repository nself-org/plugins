package internal

import (
	"os"
	"testing"
)

// Purpose: unit tests for LoadConfig.
// Inputs: environment variables CONTENT_SAFETY_PLUGIN_PORT, CONTENT_SAFETY_PLUGIN_HOST,
//
//	DATABASE_URL, CONTENT_SAFETY_API_KEY.
//
// Outputs: Config struct with expected field values.
// Constraints: no external deps, no DB — pure env-var parsing.
func TestLoadConfigDefaults(t *testing.T) {
	// Clear relevant env vars so we get pure defaults.
	os.Unsetenv("CONTENT_SAFETY_PLUGIN_PORT")
	os.Unsetenv("CONTENT_SAFETY_PLUGIN_HOST")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("CONTENT_SAFETY_API_KEY")

	cfg := LoadConfig()

	if cfg.Port != 3213 {
		t.Errorf("default port: got %d, want 3213", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("default host: got %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("default DatabaseURL: got %q, want empty", cfg.DatabaseURL)
	}
	if cfg.APIKey != "" {
		t.Errorf("default APIKey: got %q, want empty", cfg.APIKey)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	os.Setenv("CONTENT_SAFETY_PLUGIN_PORT", "9090")
	os.Setenv("CONTENT_SAFETY_PLUGIN_HOST", "127.0.0.1")
	os.Setenv("DATABASE_URL", "postgres://localhost/test")
	os.Setenv("CONTENT_SAFETY_API_KEY", "test-key-abc")
	defer func() {
		os.Unsetenv("CONTENT_SAFETY_PLUGIN_PORT")
		os.Unsetenv("CONTENT_SAFETY_PLUGIN_HOST")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("CONTENT_SAFETY_API_KEY")
	}()

	cfg := LoadConfig()

	if cfg.Port != 9090 {
		t.Errorf("port from env: got %d, want 9090", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("host from env: got %q, want %q", cfg.Host, "127.0.0.1")
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("DatabaseURL from env: got %q", cfg.DatabaseURL)
	}
	if cfg.APIKey != "test-key-abc" {
		t.Errorf("APIKey from env: got %q", cfg.APIKey)
	}
}

func TestLoadConfigInvalidPort(t *testing.T) {
	os.Setenv("CONTENT_SAFETY_PLUGIN_PORT", "not-a-number")
	defer os.Unsetenv("CONTENT_SAFETY_PLUGIN_PORT")

	cfg := LoadConfig()
	// Invalid port string falls back to default.
	if cfg.Port != 3213 {
		t.Errorf("invalid port fallback: got %d, want 3213", cfg.Port)
	}
}
