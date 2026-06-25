package internal

import (
	"strings"
	"testing"
)

// ── T06 — Env credential exposure hardening ───────────────────────────────────

// TestRunnerEnv_ForbiddenKeysAbsent verifies that secret-bearing environment
// variables are stripped from the subprocess environment passed to the GitHub
// Actions runner. The runner executes untrusted workflow code; inheriting
// HASURA_ADMIN_SECRET, DATABASE_URL, or NSELF_LICENSE_PRIV_HEX would allow
// exfiltration by a malicious workflow step.
//
// Security context: previously the handler called os.Environ() directly,
// forwarding all process variables to the runner subprocess. The fix uses
// runnerEnv(), which applies a hard deny-list and an explicit allowlist.
func TestRunnerEnv_ForbiddenKeysAbsent(t *testing.T) {
	t.Setenv("HASURA_ADMIN_SECRET", "secret-admin-password")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("NSELF_LICENSE_PRIV_HEX", "deadbeefcafe123456789abcdef")
	t.Setenv("STRIPE_SECRET_KEY", "sk_live_aaabbbccc")
	t.Setenv("MY_APP_PASSWORD", "hunter2")
	t.Setenv("SOME_PRIVATE_KEY", "-----BEGIN PRIVATE KEY-----")

	env := runnerEnv()

	returned := make(map[string]bool, len(env))
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq >= 0 {
			returned[kv[:eq]] = true
		}
	}

	forbidden := []string{
		"HASURA_ADMIN_SECRET",
		"DATABASE_URL",
		"NSELF_LICENSE_PRIV_HEX",
		"STRIPE_SECRET_KEY",
		"MY_APP_PASSWORD",
		"SOME_PRIVATE_KEY",
	}
	for _, key := range forbidden {
		if returned[key] {
			t.Errorf("runnerEnv() leaked forbidden key %q — runner subprocess must never receive credentials", key)
		}
	}
}

// TestRunnerEnv_SafeKeysPresent verifies that non-secret system environment
// variables (PATH, HOME, USER) are forwarded so the runner can function.
func TestRunnerEnv_SafeKeysPresent(t *testing.T) {
	t.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin")
	t.Setenv("HOME", "/root")
	t.Setenv("USER", "runner")

	env := runnerEnv()
	returned := make(map[string]bool, len(env))
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq >= 0 {
			returned[kv[:eq]] = true
		}
	}

	for _, key := range []string{"PATH", "HOME", "USER"} {
		if !returned[key] {
			t.Errorf("runnerEnv() dropped required key %q — runner may fail to find executables", key)
		}
	}
}

// TestRunnerEnv_DenyBeatsAllow verifies that the hard deny list wins over the
// prefix allowlist. RUNNER_SECRET matches the RUNNER_ prefix allowlist but
// also matches the SECRET deny substring — deny must win.
func TestRunnerEnv_DenyBeatsAllow(t *testing.T) {
	t.Setenv("RUNNER_SECRET", "should-be-stripped")

	env := runnerEnv()
	for _, kv := range env {
		if strings.HasPrefix(kv, "RUNNER_SECRET=") {
			t.Error("runnerEnv() forwarded RUNNER_SECRET — deny-beats-allow rule violated")
		}
	}
}

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
