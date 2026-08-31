package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestLoadConfig_Defaults verifies LoadConfig returns documented default values
// when no env vars are set.
func TestLoadConfig_Defaults(t *testing.T) {
	keys := []string{
		"ACL_PLUGIN_PORT",
		"DATABASE_URL",
		"ACL_CACHE_TTL_SECONDS",
		"ACL_MAX_ROLE_DEPTH",
		"ACL_DEFAULT_DENY",
		"ACL_API_KEY",
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
	if c.Port != 3027 {
		t.Errorf("default Port = %d; want 3027", c.Port)
	}
	if c.DatabaseURL != "" {
		t.Errorf("default DatabaseURL = %q; want empty", c.DatabaseURL)
	}
	if c.CacheTTLSeconds != 300 {
		t.Errorf("default CacheTTLSeconds = %d; want 300", c.CacheTTLSeconds)
	}
	if c.MaxRoleDepth != 10 {
		t.Errorf("default MaxRoleDepth = %d; want 10", c.MaxRoleDepth)
	}
	if c.DefaultDeny != true {
		t.Errorf("default DefaultDeny = %v; want true", c.DefaultDeny)
	}
	if c.APIKey != "" {
		t.Errorf("default APIKey = %q; want empty", c.APIKey)
	}
}

// TestLoadConfig_EnvOverrides verifies env vars override defaults.
func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("ACL_PLUGIN_PORT", "9999")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("ACL_CACHE_TTL_SECONDS", "120")
	os.Setenv("ACL_MAX_ROLE_DEPTH", "25")
	os.Setenv("ACL_DEFAULT_DENY", "false")
	os.Setenv("ACL_API_KEY", "secret")
	defer func() {
		for _, k := range []string{
			"ACL_PLUGIN_PORT", "DATABASE_URL", "ACL_CACHE_TTL_SECONDS",
			"ACL_MAX_ROLE_DEPTH", "ACL_DEFAULT_DENY", "ACL_API_KEY",
		} {
			os.Unsetenv(k)
		}
	}()

	c := LoadConfig()
	if c.Port != 9999 || c.DatabaseURL != "postgres://x" ||
		c.CacheTTLSeconds != 120 || c.MaxRoleDepth != 25 ||
		c.DefaultDeny != false || c.APIKey != "secret" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

// TestEnvInt_InvalidFallsBack verifies non-numeric env values fall back to defaults.
func TestEnvInt_InvalidFallsBack(t *testing.T) {
	os.Setenv("ACL_PLUGIN_PORT", "notanumber")
	defer os.Unsetenv("ACL_PLUGIN_PORT")
	if LoadConfig().Port != 3027 {
		t.Error("invalid env should fall back to default port 3027")
	}
}

// TestEnvBool_AllForms verifies envBool handles all documented true/false forms.
func TestEnvBool_AllForms(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
	}
	for _, tc := range cases {
		t.Run(tc.val, func(t *testing.T) {
			os.Setenv("ACL_DEFAULT_DENY", tc.val)
			defer os.Unsetenv("ACL_DEFAULT_DENY")
			if got := LoadConfig().DefaultDeny; got != tc.want {
				t.Errorf("envBool(%q) = %v; want %v", tc.val, got, tc.want)
			}
		})
	}
}

// TestEnvBool_GarbageReturnsDefault verifies unknown values fall back to default.
func TestEnvBool_GarbageReturnsDefault(t *testing.T) {
	os.Setenv("ACL_DEFAULT_DENY", "maybe")
	defer os.Unsetenv("ACL_DEFAULT_DENY")
	if !LoadConfig().DefaultDeny {
		t.Error("unknown bool value should fall back to default true")
	}
}

// TestEnvStr_Empty verifies empty env returns default.
func TestEnvStr_Empty(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	if LoadConfig().DatabaseURL != "" {
		t.Error("unset DATABASE_URL should yield empty string")
	}
}

// --- helpers (handlers.go) ---

func TestSA_HeaderPresent(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-Id", "acct-42")
	if got := sa(r); got != "acct-42" {
		t.Errorf("sa = %q; want acct-42", got)
	}
}

func TestSA_HeaderMissing(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if got := sa(r); got != "primary" {
		t.Errorf("sa default = %q; want primary", got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]int{"n": 7})
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d; want 201", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("missing content-type; got %q", rec.Header().Get("Content-Type"))
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

func TestRawJSON_NilOrEmpty(t *testing.T) {
	if string(rawJSON(nil)) != "{}" {
		t.Error("rawJSON(nil) should return {}")
	}
	if string(rawJSON(json.RawMessage(""))) != "{}" {
		t.Error("rawJSON(empty) should return {}")
	}
	if string(rawJSON(json.RawMessage(`{"a":1}`))) != `{"a":1}` {
		t.Error("rawJSON(non-empty) should pass through")
	}
}

func TestBoolPtr(t *testing.T) {
	p := boolPtr(true)
	if p == nil || *p != true {
		t.Error("boolPtr(true) should return *bool=true")
	}
	p = boolPtr(false)
	if p == nil || *p != false {
		t.Error("boolPtr(false) should return *bool=false")
	}
}
