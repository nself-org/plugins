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
		"PORT", "HOST", "DATABASE_URL",
		"DEV_ENROLLMENT_TOKEN_TTL", "DEV_CHALLENGE_TTL",
		"DEV_HEARTBEAT_INTERVAL", "DEV_HEARTBEAT_TIMEOUT",
		"DEV_COMMAND_DEFAULT_TIMEOUT", "DEV_COMMAND_MAX_RETRIES",
		"DEV_TELEMETRY_RETENTION_DAYS",
		"DEV_INGEST_HEARTBEAT_INTERVAL", "DEV_INGEST_HEARTBEAT_TIMEOUT",
		"DEV_INGEST_RETRY_MAX",
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
	if c.Port != 3603 {
		t.Errorf("default Port = %d; want 3603", c.Port)
	}
	if c.Host != "0.0.0.0" {
		t.Errorf("default Host = %q", c.Host)
	}
	if c.EnrollmentTokenTTL != 3600 || c.ChallengeTTL != 300 ||
		c.HeartbeatInterval != 30 || c.HeartbeatTimeout != 90 ||
		c.CommandDefaultTimeout != 60 || c.CommandMaxRetries != 3 ||
		c.TelemetryRetentionDays != 30 || c.IngestHeartbeatInterval != 15 ||
		c.IngestHeartbeatTimeout != 45 || c.IngestRetryMax != 5 {
		t.Errorf("defaults wrong: %+v", c)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9999")
	os.Setenv("HOST", "127.0.0.1")
	os.Setenv("DEV_HEARTBEAT_INTERVAL", "10")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("HOST")
		os.Unsetenv("DEV_HEARTBEAT_INTERVAL")
	}()
	c := LoadConfig()
	if c.Port != 9999 || c.Host != "127.0.0.1" || c.HeartbeatInterval != 10 {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestEnvStr(t *testing.T) {
	os.Unsetenv("DEV_TEST_KEY")
	if envStr("DEV_TEST_KEY", "default") != "default" {
		t.Error("envStr should return default when unset")
	}
	os.Setenv("DEV_TEST_KEY", "actual")
	defer os.Unsetenv("DEV_TEST_KEY")
	if envStr("DEV_TEST_KEY", "default") != "actual" {
		t.Error("envStr should return env value when set")
	}
}

func TestEnvInt(t *testing.T) {
	os.Unsetenv("DEV_TEST_INT")
	if envInt("DEV_TEST_INT", 42) != 42 {
		t.Error("envInt should return default when unset")
	}
	os.Setenv("DEV_TEST_INT", "100")
	defer os.Unsetenv("DEV_TEST_INT")
	if envInt("DEV_TEST_INT", 42) != 100 {
		t.Error("envInt should parse env value")
	}
	os.Setenv("DEV_TEST_INT", "x")
	if envInt("DEV_TEST_INT", 42) != 42 {
		t.Error("envInt should fall back on parse error")
	}
}

func TestSA(t *testing.T) {
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
	var out map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
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
}
