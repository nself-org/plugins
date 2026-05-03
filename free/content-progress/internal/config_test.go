package internal

import (
	"testing"
)

// TestLoadConfig_Defaults verifies that LoadConfig returns sane defaults when
// no environment variables are set.
func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("PROGRESS_PLUGIN_PORT", "")
	t.Setenv("PORT", "")
	t.Setenv("PROGRESS_COMPLETE_THRESHOLD", "")
	t.Setenv("PROGRESS_HISTORY_SAMPLE_SECONDS", "")
	t.Setenv("PROGRESS_HISTORY_RETENTION_DAYS", "")

	cfg := LoadConfig()
	if cfg.Port != 3022 {
		t.Errorf("default Port = %d, want 3022", cfg.Port)
	}
	if cfg.CompleteThreshold != 95 {
		t.Errorf("default CompleteThreshold = %d, want 95", cfg.CompleteThreshold)
	}
	if cfg.HistorySampleSeconds != 30 {
		t.Errorf("default HistorySampleSeconds = %d, want 30", cfg.HistorySampleSeconds)
	}
	if cfg.HistoryRetentionDays != 365 {
		t.Errorf("default HistoryRetentionDays = %d, want 365", cfg.HistoryRetentionDays)
	}
}

// TestLoadConfig_CustomValues verifies that environment variables override the
// defaults.
func TestLoadConfig_CustomValues(t *testing.T) {
	t.Setenv("PROGRESS_PLUGIN_PORT", "9090")
	t.Setenv("PROGRESS_COMPLETE_THRESHOLD", "80")
	t.Setenv("PROGRESS_HISTORY_SAMPLE_SECONDS", "10")
	t.Setenv("PROGRESS_HISTORY_RETENTION_DAYS", "90")

	cfg := LoadConfig()
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.CompleteThreshold != 80 {
		t.Errorf("CompleteThreshold = %d, want 80", cfg.CompleteThreshold)
	}
	if cfg.HistorySampleSeconds != 10 {
		t.Errorf("HistorySampleSeconds = %d, want 10", cfg.HistorySampleSeconds)
	}
	if cfg.HistoryRetentionDays != 90 {
		t.Errorf("HistoryRetentionDays = %d, want 90", cfg.HistoryRetentionDays)
	}
}

// TestLoadConfig_PortFallback verifies that the PORT env var is used as a
// fallback when PROGRESS_PLUGIN_PORT is not set.
func TestLoadConfig_PortFallback(t *testing.T) {
	t.Setenv("PROGRESS_PLUGIN_PORT", "")
	t.Setenv("PORT", "8080")
	cfg := LoadConfig()
	if cfg.Port != 8080 {
		t.Errorf("Port (fallback) = %d, want 8080", cfg.Port)
	}
}

// TestValidate_Valid verifies that a valid config passes validation.
func TestValidate_Valid(t *testing.T) {
	cfg := Config{
		CompleteThreshold:    95,
		HistorySampleSeconds: 30,
		HistoryRetentionDays: 365,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

// TestValidate_InvalidThreshold verifies that an out-of-range
// CompleteThreshold fails validation.
func TestValidate_InvalidThreshold(t *testing.T) {
	cases := []int{0, 101, -1}
	for _, v := range cases {
		cfg := Config{
			CompleteThreshold:    v,
			HistorySampleSeconds: 30,
			HistoryRetentionDays: 365,
		}
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate() expected error for CompleteThreshold=%d, got nil", v)
		}
	}
}

// TestValidate_InvalidSampleSeconds verifies that a zero or negative
// HistorySampleSeconds fails validation.
func TestValidate_InvalidSampleSeconds(t *testing.T) {
	cfg := Config{
		CompleteThreshold:    95,
		HistorySampleSeconds: 0,
		HistoryRetentionDays: 365,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for HistorySampleSeconds=0, got nil")
	}
}

// TestValidate_InvalidRetentionDays verifies that a zero HistoryRetentionDays
// fails validation.
func TestValidate_InvalidRetentionDays(t *testing.T) {
	cfg := Config{
		CompleteThreshold:    95,
		HistorySampleSeconds: 30,
		HistoryRetentionDays: 0,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for HistoryRetentionDays=0, got nil")
	}
}
