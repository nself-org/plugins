// Purpose: Baseline tests for the rom-discovery plugin internal package.
// Inputs: LoadConfig() env defaults, struct types.
// Outputs: Config struct defaults, compiled package.
// Constraints: No DB or HTTP calls.
package internal

import "testing"

// TestBuild verifies the rom-discovery internal package compiles.
func TestBuild(t *testing.T) {
	t.Log("rom-discovery internal package compiled OK")
}

// TestLoadConfigHasDefaults verifies LoadConfig returns sensible defaults.
func TestLoadConfigHasDefaults(t *testing.T) {
	cfg := LoadConfig()
	if cfg.Port == "" {
		t.Error("LoadConfig().Port should have a default")
	}
	if cfg.DatabaseURL == "" {
		t.Error("LoadConfig().DatabaseURL should have a default")
	}
}
