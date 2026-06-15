package internal

import (
	"os"
	"strings"
	"testing"
)

// TestBaseURL_Sandbox verifies that the sandbox environment returns the
// sandbox API base URL.
func TestBaseURL_Sandbox(t *testing.T) {
	cfg := &Config{Environment: "sandbox"}
	got := cfg.BaseURL()
	want := "https://api-m.sandbox.paypal.com"
	if got != want {
		t.Errorf("BaseURL(sandbox) = %q, want %q", got, want)
	}
}

// TestBaseURL_Live verifies that the live environment returns the live API
// base URL.
func TestBaseURL_Live(t *testing.T) {
	cfg := &Config{Environment: "live"}
	got := cfg.BaseURL()
	want := "https://api-m.paypal.com"
	if got != want {
		t.Errorf("BaseURL(live) = %q, want %q", got, want)
	}
}

// TestBaseURL_Default verifies that any non-"live" environment returns the
// sandbox URL (safe default).
func TestBaseURL_Default(t *testing.T) {
	cfg := &Config{Environment: ""}
	got := cfg.BaseURL()
	want := "https://api-m.sandbox.paypal.com"
	if got != want {
		t.Errorf("BaseURL(empty) = %q, want %q", got, want)
	}
}

// TestSplitCSV verifies that splitCSV correctly tokenises comma-separated
// strings and handles edge cases.
func TestSplitCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b , c  ", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"a,,b", []string{"a", "b"}}, // empty segment skipped
		{"", nil},
		{"  ,  ,  ", nil}, // all whitespace-only segments skipped
	}
	for _, tc := range cases {
		got := splitCSV(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("splitCSV(%q) = %v (len %d), want %v (len %d)",
				tc.input, got, len(got), tc.want, len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

// TestLoadConfig_MismatchedCounts verifies that LoadConfig returns an error
// when PAYPAL_CLIENT_IDS and PAYPAL_CLIENT_SECRETS have different CSV lengths.
func TestLoadConfig_MismatchedCounts(t *testing.T) {
	// Save original environment.
	originalIDs := os.Getenv("PAYPAL_CLIENT_IDS")
	originalSecrets := os.Getenv("PAYPAL_CLIENT_SECRETS")
	defer func() {
		os.Setenv("PAYPAL_CLIENT_IDS", originalIDs)
		os.Setenv("PAYPAL_CLIENT_SECRETS", originalSecrets)
	}()

	// Test: 3 IDs, 2 secrets.
	os.Setenv("PAYPAL_CLIENT_IDS", "id1,id2,id3")
	os.Setenv("PAYPAL_CLIENT_SECRETS", "sec1,sec2")
	cfg, err := LoadConfig()
	if err == nil {
		t.Errorf("LoadConfig with 3 IDs and 2 secrets should return error, got nil")
	}
	if cfg != nil {
		t.Errorf("LoadConfig with mismatch should return nil config, got %v", cfg)
	}
	if err != nil && (len(err.Error()) == 0 || !strings.Contains(err.Error(), "3") || !strings.Contains(err.Error(), "2")) {
		t.Errorf("error message should contain counts (3 and 2), got: %v", err)
	}
}

// TestLoadConfig_EmptyIDsMismatch verifies that LoadConfig returns an error
// when PAYPAL_CLIENT_SECRETS is set but PAYPAL_CLIENT_IDS is empty.
func TestLoadConfig_EmptyIDsMismatch(t *testing.T) {
	// Save original environment.
	originalIDs := os.Getenv("PAYPAL_CLIENT_IDS")
	originalSecrets := os.Getenv("PAYPAL_CLIENT_SECRETS")
	defer func() {
		os.Setenv("PAYPAL_CLIENT_IDS", originalIDs)
		os.Setenv("PAYPAL_CLIENT_SECRETS", originalSecrets)
	}()

	// Test: 0 IDs, 2 secrets.
	os.Setenv("PAYPAL_CLIENT_IDS", "")
	os.Setenv("PAYPAL_CLIENT_SECRETS", "sec1,sec2")
	cfg, err := LoadConfig()
	if err == nil {
		t.Errorf("LoadConfig with 0 IDs and 2 secrets should return error, got nil")
	}
	if cfg != nil {
		t.Errorf("LoadConfig with mismatch should return nil config, got %v", cfg)
	}
}

// TestLoadConfig_HappyPath verifies that LoadConfig succeeds when
// PAYPAL_CLIENT_IDS and PAYPAL_CLIENT_SECRETS have matching counts.
func TestLoadConfig_HappyPath(t *testing.T) {
	// Save original environment.
	originalIDs := os.Getenv("PAYPAL_CLIENT_IDS")
	originalSecrets := os.Getenv("PAYPAL_CLIENT_SECRETS")
	originalLabels := os.Getenv("PAYPAL_ACCOUNT_LABELS")
	defer func() {
		os.Setenv("PAYPAL_CLIENT_IDS", originalIDs)
		os.Setenv("PAYPAL_CLIENT_SECRETS", originalSecrets)
		os.Setenv("PAYPAL_ACCOUNT_LABELS", originalLabels)
	}()

	// Test: 2 IDs, 2 secrets.
	os.Setenv("PAYPAL_CLIENT_IDS", "id1,id2")
	os.Setenv("PAYPAL_CLIENT_SECRETS", "sec1,sec2")
	os.Setenv("PAYPAL_ACCOUNT_LABELS", "acct1,acct2")
	cfg, err := LoadConfig()
	if err != nil {
		t.Errorf("LoadConfig with matching counts should not error, got: %v", err)
	}
	if cfg == nil {
		t.Errorf("LoadConfig should return non-nil config")
	}
	if cfg != nil && len(cfg.Accounts) != 2 {
		t.Errorf("LoadConfig should create 2 accounts, got %d", len(cfg.Accounts))
	}
	if cfg != nil && len(cfg.Accounts) >= 2 {
		if cfg.Accounts[0].ClientID != "id1" || cfg.Accounts[0].ClientSecret != "sec1" {
			t.Errorf("First account mismatch: got id=%q sec=%q", cfg.Accounts[0].ClientID, cfg.Accounts[0].ClientSecret)
		}
		if cfg.Accounts[1].ClientID != "id2" || cfg.Accounts[1].ClientSecret != "sec2" {
			t.Errorf("Second account mismatch: got id=%q sec=%q", cfg.Accounts[1].ClientID, cfg.Accounts[1].ClientSecret)
		}
	}
}
