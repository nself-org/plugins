package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	keys := []string{
		"PORT", "DATABASE_URL", "IDME_CLIENT_ID",
		"IDME_CLIENT_SECRET", "IDME_REDIRECT_URI",
		"IDME_SCOPES", "IDME_SANDBOX", "IDME_WEBHOOK_SECRET",
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
		t.Errorf("default Port = %q; want %s", c.Port, DefaultPort)
	}
	if c.Scopes != "openid,email,profile" {
		t.Errorf("default Scopes = %q", c.Scopes)
	}
	if c.Sandbox {
		t.Error("default Sandbox should be false")
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9999")
	os.Setenv("IDME_SCOPES", "openid,extra")
	os.Setenv("IDME_SANDBOX", "true")
	os.Setenv("IDME_CLIENT_ID", "cid")
	defer func() {
		for _, k := range []string{
			"PORT", "IDME_SCOPES", "IDME_SANDBOX", "IDME_CLIENT_ID",
		} {
			os.Unsetenv(k)
		}
	}()
	c := LoadConfig()
	if c.Port != "9999" || c.Scopes != "openid,extra" || !c.Sandbox || c.ClientID != "cid" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestSandbox_OnlyTrueLiteral(t *testing.T) {
	for _, val := range []string{"1", "yes", "TRUE", ""} {
		t.Run(val, func(t *testing.T) {
			if val == "" {
				os.Unsetenv("IDME_SANDBOX")
			} else {
				os.Setenv("IDME_SANDBOX", val)
				defer os.Unsetenv("IDME_SANDBOX")
			}
			if LoadConfig().Sandbox {
				t.Errorf("Sandbox should require literal \"true\"; %q enabled it", val)
			}
		})
	}
}

func TestSA(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if sa(r) != "primary" {
		t.Error("sa default should be primary")
	}
	r.Header.Set("X-Source-Account-Id", "acct-1")
	if sa(r) != "acct-1" {
		t.Error("sa should pick up header")
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
