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
		"CDN_PLUGIN_PORT", "PORT", "CDN_PLUGIN_HOST",
		"CDN_SIGNED_URL_TTL", "CDN_PURGE_BATCH_SIZE",
		"DATABASE_URL", "CDN_API_KEY", "CDN_SIGNING_KEY",
		"CDN_CLOUDFLARE_API_TOKEN", "CDN_BUNNYCDN_API_KEY",
		"CDN_FASTLY_API_TOKEN",
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
		t.Errorf("default Host = %q", c.Host)
	}
	if c.SignedURLTTL != 3600 {
		t.Errorf("default SignedURLTTL = %d; want 3600", c.SignedURLTTL)
	}
	if c.PurgeBatchSize != 30 {
		t.Errorf("default PurgeBatchSize = %d; want 30", c.PurgeBatchSize)
	}
}

func TestLoadConfig_PortFallbackToPort(t *testing.T) {
	os.Unsetenv("CDN_PLUGIN_PORT")
	os.Setenv("PORT", "9090")
	defer os.Unsetenv("PORT")
	if LoadConfig().Port != 9090 {
		t.Error("PORT fallback should apply when CDN_PLUGIN_PORT is unset")
	}
}

func TestLoadConfig_CDNPortWinsOverPORT(t *testing.T) {
	os.Setenv("CDN_PLUGIN_PORT", "1111")
	os.Setenv("PORT", "2222")
	defer func() {
		os.Unsetenv("CDN_PLUGIN_PORT")
		os.Unsetenv("PORT")
	}()
	if LoadConfig().Port != 1111 {
		t.Error("CDN_PLUGIN_PORT should win over PORT")
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("CDN_PLUGIN_PORT", "9999")
	os.Setenv("CDN_PLUGIN_HOST", "127.0.0.1")
	os.Setenv("CDN_SIGNED_URL_TTL", "60")
	os.Setenv("CDN_PURGE_BATCH_SIZE", "5")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("CDN_SIGNING_KEY", "k")
	defer func() {
		for _, k := range []string{
			"CDN_PLUGIN_PORT", "CDN_PLUGIN_HOST",
			"CDN_SIGNED_URL_TTL", "CDN_PURGE_BATCH_SIZE",
			"DATABASE_URL", "CDN_SIGNING_KEY",
		} {
			os.Unsetenv(k)
		}
	}()

	c := LoadConfig()
	if c.Port != 9999 || c.Host != "127.0.0.1" ||
		c.SignedURLTTL != 60 || c.PurgeBatchSize != 5 ||
		c.DatabaseURL != "postgres://x" || c.SigningKey != "k" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestSA_Normalises(t *testing.T) {
	cases := []struct {
		header, want string
	}{
		{"", "primary"},
		{"Acct42", "acct42"},
		{"Foo Bar!", "foo-bar"},
		{"!!!!", "primary"},
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
	if out["status"] != "ok" || out["plugin"] != "cdn" {
		t.Errorf("body = %v", out)
	}
}

func TestGenerateSignedURL_AppendsExpiresAndSig(t *testing.T) {
	got := generateSignedURL("https://x.com/path", "key", 60, "")
	if !strings.Contains(got, "expires=") || !strings.Contains(got, "sig=") {
		t.Errorf("missing expires or sig: %q", got)
	}
	if !strings.HasPrefix(got, "https://x.com/path?") {
		t.Errorf("expected ? separator: %q", got)
	}
}

func TestGenerateSignedURL_WithExistingQuery(t *testing.T) {
	got := generateSignedURL("https://x.com/p?a=1", "key", 60, "")
	if !strings.Contains(got, "?a=1&expires=") {
		t.Errorf("expected & separator after existing query: %q", got)
	}
}

func TestGenerateSignedURL_WithIP(t *testing.T) {
	got := generateSignedURL("https://x.com/p", "key", 60, "10.0.0.1")
	if !strings.Contains(got, "&ip=10.0.0.1") {
		t.Errorf("ip param missing: %q", got)
	}
}
