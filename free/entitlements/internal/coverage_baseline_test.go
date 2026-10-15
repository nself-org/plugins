package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")

	c := LoadConfig()
	if c.Port != "3735" {
		t.Errorf("default Port = %q; want 3735", c.Port)
	}
	if !strings.HasPrefix(c.DatabaseURL, "postgres://") {
		t.Errorf("default DatabaseURL = %q", c.DatabaseURL)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DATABASE_URL", "postgres://x")
	c := LoadConfig()
	if c.Port != "9999" || c.DatabaseURL != "postgres://x" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestSA_Header(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-ID", "acct-7")
	if got := h.sa(r); got != "acct-7" {
		t.Errorf("sa = %q", got)
	}
}

func TestSA_Default(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	if got := h.sa(r); got != "primary" {
		t.Errorf("sa default = %q", got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"k": "v"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("content-type missing")
	}
}

func TestHealth(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	h.Health(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
}
