package internal

import (
	"strings"
	"testing"
)

// TestTailLines_FewerThanN verifies that fewer lines than n returns all lines.
func TestTailLines_FewerThanN(t *testing.T) {
	input := "line1\nline2\nline3"
	r := strings.NewReader(input)
	got, err := tailLines(r, 10)
	if err != nil {
		t.Fatalf("tailLines error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
	if got[0] != "line1" || got[2] != "line3" {
		t.Errorf("unexpected lines: %v", got)
	}
}

// TestTailLines_ExactlyN verifies that exactly n lines returns all lines.
func TestTailLines_ExactlyN(t *testing.T) {
	input := "a\nb\nc"
	r := strings.NewReader(input)
	got, err := tailLines(r, 3)
	if err != nil {
		t.Fatalf("tailLines error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3", len(got))
	}
}

// TestTailLines_MoreThanN verifies that only the last n lines are returned.
func TestTailLines_MoreThanN(t *testing.T) {
	lines := []string{"one", "two", "three", "four", "five"}
	input := strings.Join(lines, "\n")
	r := strings.NewReader(input)
	got, err := tailLines(r, 2)
	if err != nil {
		t.Fatalf("tailLines error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2", len(got))
	}
	if got[0] != "four" || got[1] != "five" {
		t.Errorf("last 2 lines = %v, want [four five]", got)
	}
}

// TestTailLines_EmptyInput verifies that empty input returns an empty slice.
func TestTailLines_EmptyInput(t *testing.T) {
	r := strings.NewReader("")
	got, err := tailLines(r, 5)
	if err != nil {
		t.Fatalf("tailLines error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

// TestTailLines_ZeroN verifies that n=0 returns an empty slice.
func TestTailLines_ZeroN(t *testing.T) {
	r := strings.NewReader("line1\nline2")
	got, err := tailLines(r, 0)
	if err != nil {
		t.Fatalf("tailLines error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice for n=0, got %v", got)
	}
}

// TestRunnerDir_Default verifies that runnerDir returns a non-empty path when
// GITHUB_RUNNER_DIR is not set.
func TestRunnerDir_Default(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_DIR", "")
	got := runnerDir()
	if got == "" {
		t.Error("runnerDir() = empty, want a non-empty path")
	}
}

// TestRunnerDir_EnvOverride verifies that GITHUB_RUNNER_DIR overrides the default.
func TestRunnerDir_EnvOverride(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_DIR", "/custom/runner")
	got := runnerDir()
	if got != "/custom/runner" {
		t.Errorf("runnerDir() = %q, want %q", got, "/custom/runner")
	}
}

// TestLogFilePath_EnvOverride verifies that GITHUB_RUNNER_LOG_PATH overrides the default.
func TestLogFilePath_EnvOverride(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_LOG_PATH", "/var/log/runner.log")
	got := logFilePath()
	if got != "/var/log/runner.log" {
		t.Errorf("logFilePath() = %q, want %q", got, "/var/log/runner.log")
	}
}

// TestLogFilePath_Default verifies that logFilePath returns a non-empty path when
// GITHUB_RUNNER_LOG_PATH is not set.
func TestLogFilePath_Default(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_LOG_PATH", "")
	got := logFilePath()
	if got == "" {
		t.Error("logFilePath() = empty, want a non-empty path")
	}
}
