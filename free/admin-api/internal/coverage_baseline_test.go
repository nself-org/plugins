package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	keys := []string{
		"ADMIN_API_PLUGIN_PORT", "ADMIN_API_CACHE_TTL",
		"DB_POOL_MIN", "DB_POOL_MAX", "CORS_ALLOWED_ORIGIN",
		"DATABASE_URL", "PROMETHEUS_URL",
		"ADMIN_API_WS_ENABLED", "ADMIN_API_KEY",
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

	c := LoadConfig()
	if c.Port != DefaultPort {
		t.Errorf("default Port = %d; want %d", c.Port, DefaultPort)
	}
	if c.Host != "0.0.0.0" {
		t.Errorf("default Host = %q; want 0.0.0.0", c.Host)
	}
	if c.CacheTTLSeconds != 30 {
		t.Errorf("default CacheTTLSeconds = %d; want 30", c.CacheTTLSeconds)
	}
	if c.RetentionDays != 90 {
		t.Errorf("default RetentionDays = %d; want 90", c.RetentionDays)
	}
	if c.SnapshotIntervalM != 5 {
		t.Errorf("default SnapshotIntervalM = %d; want 5", c.SnapshotIntervalM)
	}
	if c.WSEnabled {
		t.Error("default WSEnabled should be false")
	}
	if c.CORSAllowedOrigin != "http://localhost:3021" {
		t.Errorf("default CORS = %q; want http://localhost:3021", c.CORSAllowedOrigin)
	}
	if c.DBPoolMin != 2 {
		t.Errorf("default DBPoolMin = %d; want 2", c.DBPoolMin)
	}
	if c.DBPoolMax != 10 {
		t.Errorf("default DBPoolMax = %d; want 10", c.DBPoolMax)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("ADMIN_API_PLUGIN_PORT", "9999")
	os.Setenv("ADMIN_API_CACHE_TTL", "120")
	os.Setenv("DB_POOL_MIN", "5")
	os.Setenv("DB_POOL_MAX", "50")
	os.Setenv("CORS_ALLOWED_ORIGIN", "https://example.com")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("PROMETHEUS_URL", "http://prom")
	os.Setenv("ADMIN_API_WS_ENABLED", "true")
	os.Setenv("ADMIN_API_KEY", "secret")
	defer func() {
		for _, k := range []string{
			"ADMIN_API_PLUGIN_PORT", "ADMIN_API_CACHE_TTL",
			"DB_POOL_MIN", "DB_POOL_MAX", "CORS_ALLOWED_ORIGIN",
			"DATABASE_URL", "PROMETHEUS_URL",
			"ADMIN_API_WS_ENABLED", "ADMIN_API_KEY",
		} {
			os.Unsetenv(k)
		}
	}()

	c := LoadConfig()
	if c.Port != 9999 || c.CacheTTLSeconds != 120 ||
		c.DBPoolMin != 5 || c.DBPoolMax != 50 ||
		c.CORSAllowedOrigin != "https://example.com" ||
		c.DatabaseURL != "postgres://x" || c.PrometheusURL != "http://prom" ||
		!c.WSEnabled || c.AdminAPIKey != "secret" {
		t.Errorf("env overrides not applied: %+v", c)
	}
}

func TestLoadConfig_InvalidIntsFallBack(t *testing.T) {
	os.Setenv("ADMIN_API_PLUGIN_PORT", "x")
	os.Setenv("ADMIN_API_CACHE_TTL", "x")
	os.Setenv("DB_POOL_MIN", "x")
	os.Setenv("DB_POOL_MAX", "x")
	defer func() {
		for _, k := range []string{
			"ADMIN_API_PLUGIN_PORT", "ADMIN_API_CACHE_TTL",
			"DB_POOL_MIN", "DB_POOL_MAX",
		} {
			os.Unsetenv(k)
		}
	}()

	c := LoadConfig()
	if c.Port != DefaultPort || c.CacheTTLSeconds != 30 ||
		c.DBPoolMin != 2 || c.DBPoolMax != 10 {
		t.Errorf("invalid ints did not fall back: %+v", c)
	}
}

func TestLoadConfig_WSEnabledOnlyTrueLiteral(t *testing.T) {
	for _, val := range []string{"1", "yes", "TRUE", "True", ""} {
		t.Run(val, func(t *testing.T) {
			if val == "" {
				os.Unsetenv("ADMIN_API_WS_ENABLED")
			} else {
				os.Setenv("ADMIN_API_WS_ENABLED", val)
				defer os.Unsetenv("ADMIN_API_WS_ENABLED")
			}
			if LoadConfig().WSEnabled {
				t.Errorf("WSEnabled should require literal \"true\"; %q enabled it", val)
			}
		})
	}
}

func TestSA_HeaderXSourceAccountID(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-Id", "acct-42")
	if got := sa(r); got != "acct-42" {
		t.Errorf("sa = %q; want acct-42", got)
	}
}

func TestSA_HeaderXAppID(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-App-Id", "app-7")
	if got := sa(r); got != "app-7" {
		t.Errorf("sa = %q; want app-7", got)
	}
}

func TestSA_BothHeaders_AccountIDWins(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-Id", "first")
	r.Header.Set("X-App-Id", "second")
	if got := sa(r); got != "first" {
		t.Errorf("when both set, X-Source-Account-Id should win; got %q", got)
	}
}

func TestSA_NoHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if got := sa(r); got != "primary" {
		t.Errorf("sa default = %q; want primary", got)
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
	var out map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["n"] != 7 {
		t.Errorf("body decode mismatch: %v", out)
	}
}

func TestHealthCheck(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	h.HealthCheck(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" || out["plugin"] != "admin-api" {
		t.Errorf("body = %v", out)
	}
	if _, ok := out["timestamp"].(string); !ok {
		t.Errorf("timestamp missing or wrong type: %v", out["timestamp"])
	}
}

func TestGetMetrics(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/v1/metrics", nil)
	h.GetMetrics(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("content-type missing")
	}
	// body must be a valid JSON object
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
}
