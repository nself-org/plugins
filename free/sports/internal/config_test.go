package internal

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	keys := []string{"PORT", "DATABASE_URL", "SPORTS_PROVIDER", "SPORTS_SYNC_CRON"}
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
	if c.Port != "3126" {
		t.Errorf("Port = %q; want 3126", c.Port)
	}
	if c.DatabaseURL != "postgres://postgres:postgres@localhost:5432/nself" {
		t.Errorf("DatabaseURL = %q; want default", c.DatabaseURL)
	}
	if c.Provider != "apifootball" {
		t.Errorf("Provider = %q; want apifootball", c.Provider)
	}
	if c.SyncCronEnabled {
		t.Error("SyncCronEnabled default = true; want false")
	}
}

func TestLoadConfig_Overrides(t *testing.T) {
	os.Setenv("PORT", "8000")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("SPORTS_PROVIDER", "thesportsdb")
	os.Setenv("SPORTS_SYNC_CRON", "true")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("SPORTS_PROVIDER")
		os.Unsetenv("SPORTS_SYNC_CRON")
	}()
	c := LoadConfig()
	if c.Port != "8000" {
		t.Errorf("Port = %q", c.Port)
	}
	if c.DatabaseURL != "postgres://x" {
		t.Errorf("DatabaseURL = %q", c.DatabaseURL)
	}
	if c.Provider != "thesportsdb" {
		t.Errorf("Provider = %q", c.Provider)
	}
	if !c.SyncCronEnabled {
		t.Error("SyncCronEnabled should be true")
	}
}

func TestLoadConfig_SyncCronOnlyTrueExact(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("SPORTS_SYNC_CRON", "1") // not "true" -> false
	defer os.Unsetenv("DATABASE_URL")
	defer os.Unsetenv("SPORTS_SYNC_CRON")
	c := LoadConfig()
	if c.SyncCronEnabled {
		t.Error("SyncCronEnabled with value '1' should remain false (only 'true' enables)")
	}
}
