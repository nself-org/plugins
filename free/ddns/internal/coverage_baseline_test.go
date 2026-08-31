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
	keys := []string{"DDNS_PLUGIN_PORT", "DDNS_HOST", "DATABASE_URL", "DDNS_API_KEY"}
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
		t.Errorf("default Host = %q", c.Host)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("DDNS_PLUGIN_PORT", "9999")
	os.Setenv("DDNS_HOST", "127.0.0.1")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("DDNS_API_KEY", "k")
	defer func() {
		for _, k := range []string{"DDNS_PLUGIN_PORT", "DDNS_HOST", "DATABASE_URL", "DDNS_API_KEY"} {
			os.Unsetenv(k)
		}
	}()
	c := LoadConfig()
	if c.Port != 9999 || c.Host != "127.0.0.1" || c.DatabaseURL != "postgres://x" || c.APIKey != "k" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestLoadConfig_InvalidPortFallsBack(t *testing.T) {
	os.Setenv("DDNS_PLUGIN_PORT", "x")
	defer os.Unsetenv("DDNS_PLUGIN_PORT")
	if LoadConfig().Port != DefaultPort {
		t.Error("invalid port should fall back")
	}
}

func TestSA(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "primary"},
		{"acct1", "acct1"},
		{"!!!", "primary"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/x", nil)
			if tc.in != "" {
				r.Header.Set("X-Source-Account-ID", tc.in)
			}
			if got := sa(r); got != tc.want {
				t.Errorf("sa(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]int{"a": 1})
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("bad write: %d / %q", rec.Code, rec.Header().Get("Content-Type"))
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
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Errorf("body = %v", out)
	}
}
