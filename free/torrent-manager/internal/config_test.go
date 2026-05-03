package internal

import (
	"testing"
)

// TestEnvStr_Default verifies that envStr returns the fallback when env is unset.
func TestEnvStr_Default(t *testing.T) {
	got := envStr("TORRENT_TEST_UNSET_XYZ987", "default-val")
	if got != "default-val" {
		t.Errorf("envStr = %q, want %q", got, "default-val")
	}
}

// TestEnvStr_EnvSet verifies that a set env var overrides the default.
func TestEnvStr_EnvSet(t *testing.T) {
	t.Setenv("TORRENT_TEST_KEY_1", "env-value")
	got := envStr("TORRENT_TEST_KEY_1", "default-val")
	if got != "env-value" {
		t.Errorf("envStr = %q, want %q", got, "env-value")
	}
}

// TestEnvInt_Default verifies that envInt returns the fallback when env is unset.
func TestEnvInt_Default(t *testing.T) {
	got := envInt("TORRENT_TEST_INT_UNSET_XYZ987", 9091)
	if got != 9091 {
		t.Errorf("envInt = %d, want 9091", got)
	}
}

// TestEnvInt_EnvSet verifies that a valid integer env var overrides the default.
func TestEnvInt_EnvSet(t *testing.T) {
	t.Setenv("TORRENT_TEST_INT_1", "4242")
	got := envInt("TORRENT_TEST_INT_1", 0)
	if got != 4242 {
		t.Errorf("envInt = %d, want 4242", got)
	}
}

// TestEnvInt_InvalidFallback verifies that a non-integer env var falls back to the default.
func TestEnvInt_InvalidFallback(t *testing.T) {
	t.Setenv("TORRENT_TEST_INT_BAD", "not-a-number")
	got := envInt("TORRENT_TEST_INT_BAD", 100)
	if got != 100 {
		t.Errorf("envInt with invalid value = %d, want 100", got)
	}
}

// TestEnvFloat_Default verifies that envFloat returns the fallback when env is unset.
func TestEnvFloat_Default(t *testing.T) {
	got := envFloat("TORRENT_TEST_FLOAT_UNSET_XYZ987", 2.0)
	if got != 2.0 {
		t.Errorf("envFloat = %v, want 2.0", got)
	}
}

// TestEnvFloat_EnvSet verifies that a float env var overrides the default.
func TestEnvFloat_EnvSet(t *testing.T) {
	t.Setenv("TORRENT_TEST_FLOAT_1", "1.5")
	got := envFloat("TORRENT_TEST_FLOAT_1", 0.0)
	if got != 1.5 {
		t.Errorf("envFloat = %v, want 1.5", got)
	}
}

// TestEnabledSourcesList_Default verifies the default sources CSV is split correctly.
func TestEnabledSourcesList_Default(t *testing.T) {
	cfg := &Config{EnabledSources: "1337x,yts,torrentgalaxy,tpb"}
	list := cfg.EnabledSourcesList()
	if len(list) != 4 {
		t.Errorf("len(list) = %d, want 4: %v", len(list), list)
	}
	if list[0] != "1337x" {
		t.Errorf("list[0] = %q, want %q", list[0], "1337x")
	}
}

// TestEnabledSourcesList_Empty verifies that an empty EnabledSources returns nil.
func TestEnabledSourcesList_Empty(t *testing.T) {
	cfg := &Config{EnabledSources: ""}
	list := cfg.EnabledSourcesList()
	if list != nil {
		t.Errorf("expected nil list for empty EnabledSources, got %v", list)
	}
}

// TestEnabledSourcesList_Whitespace verifies that sources with surrounding spaces are trimmed.
func TestEnabledSourcesList_Whitespace(t *testing.T) {
	cfg := &Config{EnabledSources: " 1337x , yts "}
	list := cfg.EnabledSourcesList()
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2: %v", len(list), list)
	}
	if list[0] != "1337x" || list[1] != "yts" {
		t.Errorf("unexpected trimming result: %v", list)
	}
}
