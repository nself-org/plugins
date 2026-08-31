// Purpose: Baseline tests for the tmdb plugin internal package.
// Inputs: LoadConfig() env defaults, struct type instantiation.
// Outputs: Config with non-empty defaults, struct field access.
// Constraints: No DB or HTTP calls; config and struct tests only.
package internal

import "testing"

// TestBuild verifies the tmdb internal package compiles.
func TestBuild(t *testing.T) {
	t.Log("tmdb internal package compiled OK")
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
