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
		"OS_PLUGIN_PORT", "DATABASE_URL", "OS_S3_ENDPOINT",
		"OS_S3_ACCESS_KEY", "OS_S3_SECRET_KEY",
		"OS_STORAGE_BASE_PATH", "OS_DEFAULT_PROVIDER",
		"OS_API_KEY", "OS_MAX_UPLOAD_SIZE",
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
	if c.Port != "3301" {
		t.Errorf("default Port = %q; want 3301", c.Port)
	}
	if c.StorageBasePath != "/data/object-storage" {
		t.Errorf("default StorageBasePath = %q", c.StorageBasePath)
	}
	if c.DefaultProvider != "local" {
		t.Errorf("default DefaultProvider = %q", c.DefaultProvider)
	}
	if c.MaxUploadSize != "1073741824" {
		t.Errorf("default MaxUploadSize = %q", c.MaxUploadSize)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("OS_PLUGIN_PORT", "9999")
	os.Setenv("OS_DEFAULT_PROVIDER", "s3")
	os.Setenv("OS_S3_ENDPOINT", "https://s3.example.com")
	os.Setenv("OS_MAX_UPLOAD_SIZE", "999")
	defer func() {
		for _, k := range []string{
			"OS_PLUGIN_PORT", "OS_DEFAULT_PROVIDER",
			"OS_S3_ENDPOINT", "OS_MAX_UPLOAD_SIZE",
		} {
			os.Unsetenv(k)
		}
	}()
	c := LoadConfig()
	if c.Port != "9999" || c.DefaultProvider != "s3" ||
		c.StorageEndpoint != "https://s3.example.com" ||
		c.MaxUploadSize != "999" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestGetEnv(t *testing.T) {
	os.Unsetenv("OS_TEST_KEY")
	if getEnv("OS_TEST_KEY", "default") != "default" {
		t.Error("getEnv should return default when unset")
	}
	os.Setenv("OS_TEST_KEY", "actual")
	defer os.Unsetenv("OS_TEST_KEY")
	if getEnv("OS_TEST_KEY", "default") != "actual" {
		t.Error("getEnv should return env when set")
	}
}

func TestSA_Header(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account", "acct-7")
	if sa(r) != "acct-7" {
		t.Error("sa should pick up X-Source-Account header")
	}
}

func TestSA_Default(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	if sa(r) != "primary" {
		t.Error("sa default should be primary")
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
