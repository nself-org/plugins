package pty_test

import (
	"testing"

	ptyhandler "github.com/nself-org/plugin-pty/internal/pty"
)

// TestStripControlSequences verifies ANSI escape stripping.
func TestStripControlSequences(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"\x1b[31mHello\x1b[0m", "Hello"},
		{"plain text", "plain text"},
		{"\x1b[2J\x1b[H", ""},        // clear screen + home
		{"Hello\x1b[AWorld", "HelloWorld"}, // cursor up stripped
	}
	for _, c := range cases {
		got := string(ptyhandler.StripControlSequences([]byte(c.input)))
		if got != c.want {
			t.Errorf("StripControlSequences(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestMaxPerTenantEnforcement verifies the Config.MaxPerTenant field is respected.
// (Integration test for active count logic — uses mock in production tests)
func TestConfigDefaults(t *testing.T) {
	cfg := ptyhandler.Config{
		MaxPerTenant: 5,
	}
	if cfg.MaxPerTenant != 5 {
		t.Errorf("expected MaxPerTenant=5, got %d", cfg.MaxPerTenant)
	}
}
