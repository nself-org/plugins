package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	keys := []string{
		"COMPLIANCE_PLUGIN_PORT", "COMPLIANCE_PLUGIN_HOST",
		"DATABASE_URL", "COMPLIANCE_API_KEY", "COMPLIANCE_LOG_LEVEL",
	}
	saved := make(map[string]string, len(keys))
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	defer func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	c := Load()
	if c.Port != strconv.Itoa(DefaultPort) {
		t.Errorf("default Port = %q; want %d", c.Port, DefaultPort)
	}
	if c.Host != "0.0.0.0" {
		t.Errorf("default Host = %q", c.Host)
	}
	if c.DatabaseURL != "" || c.APIKey != "" || c.LogLevel != "" {
		t.Errorf("string defaults should be empty: %+v", c)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Setenv("COMPLIANCE_PLUGIN_PORT", "9999")
	os.Setenv("COMPLIANCE_PLUGIN_HOST", "127.0.0.1")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("COMPLIANCE_API_KEY", "secret")
	os.Setenv("COMPLIANCE_LOG_LEVEL", "debug")
	defer func() {
		for _, k := range []string{
			"COMPLIANCE_PLUGIN_PORT", "COMPLIANCE_PLUGIN_HOST",
			"DATABASE_URL", "COMPLIANCE_API_KEY", "COMPLIANCE_LOG_LEVEL",
		} {
			os.Unsetenv(k)
		}
	}()

	c := Load()
	if c.Port != "9999" || c.Host != "127.0.0.1" ||
		c.DatabaseURL != "postgres://x" || c.APIKey != "secret" ||
		c.LogLevel != "debug" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestSA_Header(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-ID", "h-acct")
	if got := sa(r); got != "h-acct" {
		t.Errorf("sa from header = %q", got)
	}
}

func TestSA_QueryFallback(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?source_account_id=q-acct", nil)
	if got := sa(r); got != "q-acct" {
		t.Errorf("sa from query = %q", got)
	}
}

func TestSA_HeaderWinsOverQuery(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?source_account_id=q", nil)
	r.Header.Set("X-Source-Account-ID", "h")
	if got := sa(r); got != "h" {
		t.Errorf("header should win, got %q", got)
	}
}

func TestSA_DefaultPrimary(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if got := sa(r); got != "primary" {
		t.Errorf("default = %q; want primary", got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]int{"n": 7})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content-type missing")
	}
}

func TestErrJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	errJSON(rec, http.StatusBadRequest, "boom")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"boom"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestDecode_Valid(t *testing.T) {
	body := strings.NewReader(`{"x":1}`)
	r := httptest.NewRequest("POST", "/x", body)
	var out map[string]int
	if err := decode(r, &out); err != nil {
		t.Fatal(err)
	}
	if out["x"] != 1 {
		t.Errorf("decode = %v", out)
	}
}

func TestDecode_Invalid(t *testing.T) {
	body := strings.NewReader(`not-json`)
	r := httptest.NewRequest("POST", "/x", body)
	var out map[string]int
	if err := decode(r, &out); err == nil {
		t.Error("decode should error on invalid JSON")
	}
}

func TestHealthHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	HealthHandler(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" || out["plugin"] != "compliance" {
		t.Errorf("body = %v", out)
	}
}
