// Purpose: Tests for LoadConfig default/override behavior.
// Inputs: PORT / DATABASE_URL env vars.
// Outputs: asserts Config defaults and env overrides.
// Constraints: No DB or HTTP calls.
package internal

import (
	"os"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	saved := map[string]string{"PORT": os.Getenv("PORT"), "DATABASE_URL": os.Getenv("DATABASE_URL")}
	os.Unsetenv("PORT")
	os.Unsetenv("DATABASE_URL")
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
	if c.Port != "3128" {
		t.Errorf("default Port = %q; want 3128", c.Port)
	}
	if c.DatabaseURL != "postgres://postgres:postgres@localhost:5432/nself" {
		t.Errorf("default DatabaseURL = %q", c.DatabaseURL)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9999")
	os.Setenv("DATABASE_URL", "postgres://x")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
	}()
	c := LoadConfig()
	if c.Port != "9999" || c.DatabaseURL != "postgres://x" {
		t.Errorf("overrides not applied: %+v", c)
	}
}
