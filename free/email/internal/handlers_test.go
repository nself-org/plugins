package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceAccountID_default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := sourceAccountID(r); got != "primary" {
		t.Errorf("want primary, got %s", got)
	}
}

func TestSourceAccountID_fromHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Hasura-Source-Account-Id", "acct-abc")
	if got := sourceAccountID(r); got != "acct-abc" {
		t.Errorf("want acct-abc, got %s", got)
	}
}

func TestLoadConfig_missingDB(t *testing.T) {
	t.Setenv("NSELF_DB_URL", "")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error when NSELF_DB_URL missing")
	}
}

func TestLoadConfig_invalidPort(t *testing.T) {
	t.Setenv("NSELF_DB_URL", "postgres://localhost/test")
	t.Setenv("EMAIL_PLUGIN_PORT", "notaport")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestLoadConfig_defaults(t *testing.T) {
	t.Setenv("NSELF_DB_URL", "postgres://localhost/test")
	t.Setenv("EMAIL_PLUGIN_PORT", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9008 {
		t.Errorf("want port 9008, got %d", cfg.Port)
	}
}
