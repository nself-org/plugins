package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	keys := []string{
		"ANALYTICS_PLUGIN_PORT", "ANALYTICS_BATCH_SIZE", "ANALYTICS_HOST",
		"DATABASE_URL", "ANALYTICS_API_KEY",
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
	if c.BatchSize != 100 {
		t.Errorf("default BatchSize = %d; want 100", c.BatchSize)
	}
	if c.DatabaseURL != "" || c.APIKey != "" {
		t.Errorf("DatabaseURL and APIKey should be empty by default: %+v", c)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("ANALYTICS_PLUGIN_PORT", "9999")
	os.Setenv("ANALYTICS_BATCH_SIZE", "500")
	os.Setenv("ANALYTICS_HOST", "127.0.0.1")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("ANALYTICS_API_KEY", "secret")
	defer func() {
		for _, k := range []string{
			"ANALYTICS_PLUGIN_PORT", "ANALYTICS_BATCH_SIZE", "ANALYTICS_HOST",
			"DATABASE_URL", "ANALYTICS_API_KEY",
		} {
			os.Unsetenv(k)
		}
	}()

	c := LoadConfig()
	if c.Port != 9999 || c.BatchSize != 500 || c.Host != "127.0.0.1" ||
		c.DatabaseURL != "postgres://x" || c.APIKey != "secret" {
		t.Errorf("env overrides not applied: %+v", c)
	}
}

func TestLoadConfig_InvalidIntsFallBack(t *testing.T) {
	os.Setenv("ANALYTICS_PLUGIN_PORT", "x")
	os.Setenv("ANALYTICS_BATCH_SIZE", "x")
	defer func() {
		os.Unsetenv("ANALYTICS_PLUGIN_PORT")
		os.Unsetenv("ANALYTICS_BATCH_SIZE")
	}()

	c := LoadConfig()
	if c.Port != DefaultPort {
		t.Errorf("invalid port should fall back to %d, got %d", DefaultPort, c.Port)
	}
	if c.BatchSize != 100 {
		t.Errorf("invalid batch should fall back to 100, got %d", c.BatchSize)
	}
}

func TestSA_NormalisesInput(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", "primary"},
		{"Acct42", "acct42"},
		{"Acme Co.!", "acme-co"},
		{"!!!!", "primary"},
		{"foo_bar-baz", "foo_bar-baz"},
		{"Hello World", "hello-world"},
	}
	for _, tc := range cases {
		t.Run(tc.header, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/x", nil)
			if tc.header != "" {
				r.Header.Set("X-Source-Account-ID", tc.header)
			}
			if got := sa(r); got != tc.want {
				t.Errorf("sa(%q) = %q; want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]int{"n": 7})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("missing content-type")
	}
	var out map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["n"] != 7 {
		t.Errorf("body = %v", out)
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

func TestHandleHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	HandleHealth(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" || out["plugin"] != "analytics" {
		t.Errorf("body = %v", out)
	}
}
