package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateE164_valid(t *testing.T) {
	cases := []string{
		"+14155552671",
		"+447911123456",
		"+61412345678",
		"+12025551234",
	}
	for _, c := range cases {
		if err := ValidateE164(c); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", c, err)
		}
	}
}

func TestValidateE164_invalid(t *testing.T) {
	cases := []string{
		"14155552671",      // missing +
		"+1",              // too short
		"+0123456789",     // leading 0 in country code
		"not-a-number",
		"",
	}
	for _, c := range cases {
		if err := ValidateE164(c); err == nil {
			t.Errorf("expected %q to be invalid", c)
		}
	}
}

func TestSourceAccountID_default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := sourceAccountID(r); got != "primary" {
		t.Errorf("want primary, got %s", got)
	}
}

func TestSourceAccountID_fromHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-Source-Account-Id", "acct-xyz")
	if got := sourceAccountID(r); got != "acct-xyz" {
		t.Errorf("want acct-xyz, got %s", got)
	}
}

func TestLoadConfig_defaults(t *testing.T) {
	t.Setenv("NSELF_DB_URL", "postgres://localhost/test")
	t.Setenv("SMS_PLUGIN_PORT", "")
	t.Setenv("SMS_RATE_LIMIT_PER_MIN", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9009 {
		t.Errorf("want port 9009, got %d", cfg.Port)
	}
	if cfg.RateLimitPerMin != 10 {
		t.Errorf("want rate limit 10, got %d", cfg.RateLimitPerMin)
	}
}

func TestLoadConfig_missingDB(t *testing.T) {
	t.Setenv("NSELF_DB_URL", "")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when NSELF_DB_URL missing")
	}
}
