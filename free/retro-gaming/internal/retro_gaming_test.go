// Purpose: Baseline tests for the retro-gaming plugin internal package.
// Inputs: LoadConfig() env defaults, ROM struct type instantiation.
// Outputs: Config with non-empty port default, ROM struct field access.
// Constraints: No DB or HTTP calls; config and struct tests only.
package internal

import "testing"

// TestBuild verifies the retro-gaming internal package compiles.
func TestBuild(t *testing.T) {
	t.Log("retro-gaming internal package compiled OK")
}

// TestLoadConfigHasDefaultPort verifies LoadConfig returns a non-empty port.
func TestLoadConfigHasDefaultPort(t *testing.T) {
	cfg := LoadConfig()
	if cfg.Port == "" {
		t.Error("LoadConfig().Port should have a default")
	}
}

// TestROMStruct verifies the ROM struct can be instantiated with expected fields.
func TestROMStruct(t *testing.T) {
	r := ROM{GameTitle: "Sonic", Platform: "Genesis"}
	if r.GameTitle == "" {
		t.Error("ROM.GameTitle should not be empty after assignment")
	}
	if r.Platform == "" {
		t.Error("ROM.Platform should not be empty after assignment")
	}
}
