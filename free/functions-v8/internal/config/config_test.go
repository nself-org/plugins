package config

import (
	"os"
	"strings"
	"testing"
)

var fnEnv = []string{
	"DATABASE_URL", "FUNCTIONS_EDGE_PORT", "FUNCTIONS_EDGE_MAX_DURATION_MS",
	"FUNCTIONS_EDGE_POOL_SIZE", "FUNCTIONS_EDGE_MAX_MEMORY_MB",
	"FUNCTIONS_EDGE_LOG_RETENTION_DAYS", "DENO_BINARY_PATH",
	"PLUGIN_INTERNAL_SECRET",
}

func clearFnEnv(t *testing.T) {
	t.Helper()
	saved := make(map[string]string, len(fnEnv))
	for _, k := range fnEnv {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	})
}

func TestLoad_Defaults(t *testing.T) {
	clearFnEnv(t)
	c := Load()
	if c.Port != 3088 {
		t.Errorf("Port = %d; want 3088", c.Port)
	}
	if c.MaxDurationMS != 30000 {
		t.Errorf("MaxDurationMS = %d; want 30000", c.MaxDurationMS)
	}
	if c.PoolSize != 5 {
		t.Errorf("PoolSize = %d; want 5", c.PoolSize)
	}
	if c.MaxMemoryMB != 128 {
		t.Errorf("MaxMemoryMB = %d; want 128", c.MaxMemoryMB)
	}
	if c.LogRetentionDays != 7 {
		t.Errorf("LogRetentionDays = %d; want 7", c.LogRetentionDays)
	}
	if c.DenoBinaryPath != "/usr/bin/deno" {
		t.Errorf("DenoBinaryPath = %q; want /usr/bin/deno", c.DenoBinaryPath)
	}
}

func TestLoad_Overrides(t *testing.T) {
	clearFnEnv(t)
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("FUNCTIONS_EDGE_PORT", "9000")
	os.Setenv("FUNCTIONS_EDGE_MAX_DURATION_MS", "60000")
	os.Setenv("FUNCTIONS_EDGE_POOL_SIZE", "20")
	os.Setenv("FUNCTIONS_EDGE_MAX_MEMORY_MB", "512")
	os.Setenv("FUNCTIONS_EDGE_LOG_RETENTION_DAYS", "30")
	os.Setenv("DENO_BINARY_PATH", "/opt/deno")
	os.Setenv("PLUGIN_INTERNAL_SECRET", "s")
	c := Load()
	if c.DatabaseURL != "postgres://x" || c.Port != 9000 || c.MaxDurationMS != 60000 ||
		c.PoolSize != 20 || c.MaxMemoryMB != 512 || c.LogRetentionDays != 30 ||
		c.DenoBinaryPath != "/opt/deno" || c.InternalSecret != "s" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name   string
		c      Config
		errSub string
	}{
		{"missing db", Config{PoolSize: 1, MaxDurationMS: 1000}, "DATABASE_URL"},
		{"zero pool", Config{DatabaseURL: "x", PoolSize: 0, MaxDurationMS: 1000}, "POOL_SIZE"},
		{"low duration", Config{DatabaseURL: "x", PoolSize: 1, MaxDurationMS: 500}, "DURATION"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.c.Validate()
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Errorf("err = %v; want substring %q", err, tc.errSub)
			}
		})
	}
}

func TestValidate_OK(t *testing.T) {
	c := Config{DatabaseURL: "postgres://x", PoolSize: 5, MaxDurationMS: 1000}
	if err := c.Validate(); err != nil {
		t.Errorf("expected nil error; got %v", err)
	}
}

func TestEnvInt_InvalidFalls(t *testing.T) {
	clearFnEnv(t)
	os.Setenv("FUNCTIONS_EDGE_PORT", "not-a-number")
	c := Load()
	if c.Port != 3088 {
		t.Errorf("invalid int env should fall back to 3088; got %d", c.Port)
	}
}

func TestEnvStr_EmptyFalls(t *testing.T) {
	clearFnEnv(t)
	os.Setenv("DENO_BINARY_PATH", "")
	c := Load()
	if c.DenoBinaryPath != "/usr/bin/deno" {
		t.Errorf("empty env should use default; got %q", c.DenoBinaryPath)
	}
}
