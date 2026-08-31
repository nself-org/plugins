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
		"OBSERVABILITY_PLUGIN_PORT", "PORT", "DATABASE_URL",
		"OBSERVABILITY_CHECK_INTERVAL", "OBSERVABILITY_WATCHDOG_TIMEOUT",
		"OBSERVABILITY_HISTORY_RETAIN_DAYS",
		"OBSERVABILITY_WATCHDOG_ENABLED",
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
	if c.Port != "3215" {
		t.Errorf("default Port = %q; want 3215", c.Port)
	}
	if !strings.HasPrefix(c.DatabaseURL, "postgres://") {
		t.Errorf("default DatabaseURL = %q", c.DatabaseURL)
	}
	if c.CheckIntervalSeconds != 30 {
		t.Errorf("default CheckIntervalSeconds = %d", c.CheckIntervalSeconds)
	}
	if c.WatchdogTimeoutSeconds != 120 {
		t.Errorf("default WatchdogTimeoutSeconds = %d", c.WatchdogTimeoutSeconds)
	}
	if c.HealthHistoryRetainDays != 30 {
		t.Errorf("default HealthHistoryRetainDays = %d", c.HealthHistoryRetainDays)
	}
	if !c.WatchdogEnabled {
		t.Error("default WatchdogEnabled should be true")
	}
}

func TestLoadConfig_PortFromObservability(t *testing.T) {
	os.Setenv("OBSERVABILITY_PLUGIN_PORT", "1111")
	os.Setenv("PORT", "2222")
	defer func() {
		os.Unsetenv("OBSERVABILITY_PLUGIN_PORT")
		os.Unsetenv("PORT")
	}()
	if LoadConfig().Port != "1111" {
		t.Error("OBSERVABILITY_PLUGIN_PORT should win over PORT")
	}
}

func TestLoadConfig_PortFallbackToPort(t *testing.T) {
	os.Unsetenv("OBSERVABILITY_PLUGIN_PORT")
	os.Setenv("PORT", "2222")
	defer os.Unsetenv("PORT")
	if LoadConfig().Port != "2222" {
		t.Error("PORT should be used when OBSERVABILITY_PLUGIN_PORT unset")
	}
}

func TestLoadConfig_WatchdogDisabled(t *testing.T) {
	os.Setenv("OBSERVABILITY_WATCHDOG_ENABLED", "false")
	defer os.Unsetenv("OBSERVABILITY_WATCHDOG_ENABLED")
	if LoadConfig().WatchdogEnabled {
		t.Error("WatchdogEnabled should be false when env=false")
	}
}

func TestLoadConfig_WatchdogEnabledForOtherValues(t *testing.T) {
	for _, val := range []string{"true", "1", "yes"} {
		t.Run(val, func(t *testing.T) {
			os.Setenv("OBSERVABILITY_WATCHDOG_ENABLED", val)
			defer os.Unsetenv("OBSERVABILITY_WATCHDOG_ENABLED")
			if !LoadConfig().WatchdogEnabled {
				t.Errorf("WatchdogEnabled should be true for %q", val)
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	os.Unsetenv("OBSERVABILITY_TEST_INT")
	if envInt("OBSERVABILITY_TEST_INT", 7) != 7 {
		t.Error("envInt should return default when unset")
	}
	os.Setenv("OBSERVABILITY_TEST_INT", "42")
	defer os.Unsetenv("OBSERVABILITY_TEST_INT")
	if envInt("OBSERVABILITY_TEST_INT", 7) != 42 {
		t.Error("envInt should parse env value")
	}
	os.Setenv("OBSERVABILITY_TEST_INT", "not-a-number")
	if envInt("OBSERVABILITY_TEST_INT", 7) != 7 {
		t.Error("envInt should fall back to default when non-numeric")
	}
}

func TestEnvInt_LeadingZero(t *testing.T) {
	os.Setenv("OBSERVABILITY_TEST_INT", "007")
	defer os.Unsetenv("OBSERVABILITY_TEST_INT")
	if envInt("OBSERVABILITY_TEST_INT", 99) != 7 {
		t.Error("envInt should parse leading-zero numbers as decimal")
	}
}

func TestSA(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	if h.sa(r) != "primary" {
		t.Error("default should be primary")
	}
	r.Header.Set("X-Source-Account-ID", "acct-1")
	if h.sa(r) != "acct-1" {
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
	if out["status"] != "ok" || out["plugin"] != "observability" {
		t.Errorf("body = %v", out)
	}
}

func TestNewHandlers(t *testing.T) {
	cfg := Config{}
	h := NewHandlers(nil, cfg)
	if h == nil {
		t.Error("NewHandlers should not be nil")
	}
}
