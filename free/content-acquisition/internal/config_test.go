package internal

import (
	"testing"
)

// TestValidateURL_Valid verifies that a well-formed URL passes validation.
func TestValidateURL_Valid(t *testing.T) {
	if err := validateURL("https://example.com/path", "TEST_VAR"); err != nil {
		t.Errorf("unexpected error for valid URL: %v", err)
	}
}

// TestValidateURL_Empty verifies that an empty string fails validation.
func TestValidateURL_Empty(t *testing.T) {
	if err := validateURL("", "TEST_VAR"); err == nil {
		t.Error("expected error for empty URL, got nil")
	}
}

// TestValidateURL_NoScheme verifies that a URL without a scheme fails.
func TestValidateURL_NoScheme(t *testing.T) {
	if err := validateURL("example.com/path", "TEST_VAR"); err == nil {
		t.Error("expected error for URL without scheme, got nil")
	}
}

// TestValidateURL_ErrorContainsVarName verifies the error message includes the var name.
func TestValidateURL_ErrorContainsVarName(t *testing.T) {
	err := validateURL("not-a-url", "MY_URL_VAR")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); len(got) == 0 {
		t.Error("expected non-empty error message")
	}
}

// TestEnvOrDefault_EnvSet verifies that a set environment variable is returned.
func TestEnvOrDefault_EnvSet(t *testing.T) {
	t.Setenv("TEST_ENV_KEY_CA", "from-env")
	got := envOrDefault("TEST_ENV_KEY_CA", "default-value")
	if got != "from-env" {
		t.Errorf("envOrDefault = %q, want %q", got, "from-env")
	}
}

// TestEnvOrDefault_EnvUnset verifies that the default is returned when the env var is absent.
func TestEnvOrDefault_EnvUnset(t *testing.T) {
	t.Setenv("TEST_ENV_KEY_CA_UNSET", "")
	got := envOrDefault("TEST_ENV_KEY_CA_UNSET", "fallback")
	if got != "fallback" {
		t.Errorf("envOrDefault = %q, want %q", got, "fallback")
	}
}

// TestEnvOrDefault_EmptyUsesDefault verifies that an empty env value falls back to default.
func TestEnvOrDefault_EmptyUsesDefault(t *testing.T) {
	// Not setting the variable at all
	got := envOrDefault("TEST_ENV_KEY_CA_MISSING_XYZ987", "my-default")
	if got != "my-default" {
		t.Errorf("envOrDefault = %q, want %q", got, "my-default")
	}
}
