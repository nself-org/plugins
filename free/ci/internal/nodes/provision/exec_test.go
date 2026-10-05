package provision

// Note: LocalExecutor tests below run `echo`/`true` on whatever machine runs
// `go test`; SSHExecutor tests substitute a recording ssh stub on PATH and
// never open a real connection.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalExecutor_RunReturnsOutput(t *testing.T) {
	var ex LocalExecutor
	out, err := ex.Run(context.Background(), "echo hello-runner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello-runner" {
		t.Fatalf("out = %q, want %q", out, "hello-runner")
	}
	if ex.Label() != "local" {
		t.Fatalf("Label() = %q", ex.Label())
	}
}

func TestLocalExecutor_RunReturnsErrorOnNonZeroExit(t *testing.T) {
	var ex LocalExecutor
	if _, err := ex.Run(context.Background(), "exit 7"); err == nil {
		t.Fatal("expected an error for a non-zero exit")
	}
}

func TestLocalExecutor_SupportsNullglob(t *testing.T) {
	// Regression guard: chromium.go relies on `shopt -s nullglob` working,
	// which requires bash, not POSIX sh.
	var ex LocalExecutor
	out, err := ex.Run(context.Background(), "shopt -s nullglob; for f in /no/such/path/*; do echo FOUND; done; echo done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "done" {
		t.Fatalf("out = %q, nullglob should leave the loop empty", out)
	}
}

func TestNewSSHExecutor_DefaultsKeyPath(t *testing.T) {
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "")
	t.Setenv("NSELF_DEPLOY_SSH_KEY", "")
	ex := NewSSHExecutor("ci-user@runner-host", "")
	if !strings.HasSuffix(ex.KeyPath, "id_ed25519") {
		t.Fatalf("KeyPath = %q, want default id_ed25519", ex.KeyPath)
	}
	if ex.Label() != "ci-user@runner-host" {
		t.Fatalf("Label() = %q", ex.Label())
	}
}

func TestNewSSHExecutor_HonorsExplicitKeyPath(t *testing.T) {
	if ex := NewSSHExecutor("ci-user@runner-host", "/custom/key"); ex.KeyPath != "/custom/key" {
		t.Fatalf("KeyPath = %q, want /custom/key", ex.KeyPath)
	}
}

func TestNewSSHExecutor_UsesEnvKeyPath(t *testing.T) {
	t.Setenv("NSELF_DEPLOY_KEY_PATH", "/env/key")
	if ex := NewSSHExecutor("ci-user@runner-host", ""); ex.KeyPath != "/env/key" {
		t.Fatalf("KeyPath = %q, want /env/key", ex.KeyPath)
	}
}

// stubSSH puts a recording ssh on PATH and returns the log file path.
func stubSSH(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestSSHExecutor_ArgvAndOutput(t *testing.T) {
	log := stubSSH(t, "echo remote-out; exit 0")
	ex := NewSSHExecutor("ci@host-1", "/k/key")
	out, err := ex.Run(context.Background(), "echo hi")
	if err != nil || out != "remote-out" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	b, _ := os.ReadFile(log)
	want := "-i\n/k/key\n-o\nStrictHostKeyChecking=accept-new\n-o\nForwardAgent=no\n--\nci@host-1\necho hi\n"
	if string(b) != want {
		t.Fatalf("ssh argv:\n%q\nwant:\n%q", b, want)
	}
}

func TestSSHExecutor_FailureWrapsOutput(t *testing.T) {
	stubSSH(t, "echo boom; exit 3")
	ex := NewSSHExecutor("ci@host-1", "/k/key")
	out, err := ex.Run(context.Background(), "x")
	if err == nil || out != "boom" || !strings.Contains(err.Error(), "remote command on ci@host-1 failed") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestSSHExecutor_RefusesBeforeExec(t *testing.T) {
	log := stubSSH(t, "exit 0")
	for _, tc := range []struct{ dest, want string }{
		{"", "remote target has no SSH host configured"},
		{"-oProxyCommand=x", "must not start with '-'"},
		{"a b@host", "whitespace or a control character"},
		{"ci@host;id", "unsafe character"},
	} {
		ex := NewSSHExecutor(tc.dest, "/k")
		if _, err := ex.Run(context.Background(), "echo"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("dest %q: err=%v, want %q", tc.dest, err, tc.want)
		}
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("ssh was started for a refused destination")
	}
}
