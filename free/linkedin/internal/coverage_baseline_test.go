package internal

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLoadConfig_AllRequired(t *testing.T) {
	// All required: any missing should error.
	clearEnv := func() {
		for _, k := range []string{
			"PORT", "DATABASE_URL", "LINKEDIN_CLIENT_ID",
			"LINKEDIN_CLIENT_SECRET", "LINKEDIN_REDIRECT_URI",
			"LINKEDIN_INTERNAL_SECRET",
		} {
			os.Unsetenv(k)
		}
	}
	clearEnv()
	defer clearEnv()

	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig should error when DATABASE_URL missing")
	}
	os.Setenv("DATABASE_URL", "postgres://x")
	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig should error when LINKEDIN_CLIENT_ID missing")
	}
	os.Setenv("LINKEDIN_CLIENT_ID", "cid")
	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig should error when LINKEDIN_CLIENT_SECRET missing")
	}
	os.Setenv("LINKEDIN_CLIENT_SECRET", "sec")
	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig should error when LINKEDIN_REDIRECT_URI missing")
	}
	os.Setenv("LINKEDIN_REDIRECT_URI", "http://x")
	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig should error when LINKEDIN_INTERNAL_SECRET missing")
	}
	os.Setenv("LINKEDIN_INTERNAL_SECRET", "intsec")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should succeed when all required set: %v", err)
	}
	if cfg.Port != 3722 {
		t.Errorf("default Port = %d; want 3722", cfg.Port)
	}
}

func TestLoadConfig_PortOverride(t *testing.T) {
	for _, k := range []string{
		"DATABASE_URL", "LINKEDIN_CLIENT_ID", "LINKEDIN_CLIENT_SECRET",
		"LINKEDIN_REDIRECT_URI", "LINKEDIN_INTERNAL_SECRET",
	} {
		os.Setenv(k, "x")
	}
	os.Setenv("PORT", "9999")
	defer func() {
		for _, k := range []string{
			"PORT", "DATABASE_URL", "LINKEDIN_CLIENT_ID",
			"LINKEDIN_CLIENT_SECRET", "LINKEDIN_REDIRECT_URI",
			"LINKEDIN_INTERNAL_SECRET",
		} {
			os.Unsetenv(k)
		}
	}()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d; want 9999", cfg.Port)
	}
}

func TestLoadConfig_InvalidPortFallsBack(t *testing.T) {
	for _, k := range []string{
		"DATABASE_URL", "LINKEDIN_CLIENT_ID", "LINKEDIN_CLIENT_SECRET",
		"LINKEDIN_REDIRECT_URI", "LINKEDIN_INTERNAL_SECRET",
	} {
		os.Setenv(k, "x")
	}
	os.Setenv("PORT", "not-a-number")
	defer func() {
		for _, k := range []string{
			"PORT", "DATABASE_URL", "LINKEDIN_CLIENT_ID",
			"LINKEDIN_CLIENT_SECRET", "LINKEDIN_REDIRECT_URI",
			"LINKEDIN_INTERNAL_SECRET",
		} {
			os.Unsetenv(k)
		}
	}()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 3722 {
		t.Errorf("invalid port should fall back to 3722, got %d", cfg.Port)
	}
}

func TestSourceAccount(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if sourceAccount(r) != "primary" {
		t.Error("default should be primary")
	}
	r.Header.Set("X-Source-Account-ID", "acct-1")
	if sourceAccount(r) != "acct-1" {
		t.Error("should pick up header")
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]int{"a": 1})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("content-type missing")
	}
}

func TestGenerateState_NonEmpty(t *testing.T) {
	s, err := generateState()
	if err != nil {
		t.Fatal(err)
	}
	if s == "" {
		t.Error("generateState should not return empty")
	}
}

func TestGenerateState_Unique(t *testing.T) {
	a, _ := generateState()
	b, _ := generateState()
	if subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 {
		t.Error("two consecutive states should differ")
	}
}

func TestHandleHealth(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	h.HandleHealth(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" || out["service"] != "nself-linkedin" {
		t.Errorf("body = %v", out)
	}
}
