package legacy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretsEnvLiveKeyGate(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		passed     bool
		errorText  string
	}{
		{"absent", "", true, ""},
		{"clean", "touch env-gate-ran; exit 0", true, ""},
		{"finding", "echo env-finding; exit 1", false, "env-finding"},
		{"error", "exit 2", false, "gate error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			fakeGitleaks(t)
			if tc.body != "" {
				writeScript(t, root, "scripts/ci/env-live-key-gate.sh", tc.body)
			}
			got := runGitleaks(root, 30, false, false)
			if got.Passed != tc.passed || got.Skipped || !strings.Contains(got.Output, tc.errorText) {
				t.Fatalf("unexpected secrets verdict: %+v", got)
			}
			if tc.name == "clean" {
				if _, err := os.Stat(filepath.Join(root, "env-gate-ran")); err != nil {
					t.Fatalf("successful env gate never ran: %v", err)
				}
			}
		})
	}
}

func TestSecretsEnvLiveKeyGateTrackedValueOnly(t *testing.T) {
	root := t.TempDir()
	fakeGitleaks(t)
	for _, args := range [][]string{{"init", "-q"}, {"config", "core.hooksPath", "/dev/null"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v: %s", err, out)
		}
	}
	path := filepath.Join(root, ".environments", "prod", ".env")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture-value-without-assignment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".environments/prod/.env")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("track env fixture: %v: %s", err, out)
	}
	writeScript(t, root, "scripts/ci/env-live-key-gate.sh", `git ls-files --error-unmatch .environments/prod/.env >/dev/null || exit 2
while IFS= read -r line; do
  if [[ -n "$line" && "$line" != *=* ]]; then echo value-only-env-finding; exit 1; fi
done < .environments/prod/.env`)
	got := runGitleaks(root, 30, false, false)
	if got.Passed || got.Skipped || !strings.Contains(got.Output, "value-only-env-finding") {
		t.Fatalf("tracked value-only env must fail secrets gate: %+v", got)
	}
}

func TestSecretsEnvLiveKeyGateMissingBash(t *testing.T) {
	root := t.TempDir()
	bin := fakeGitleaks(t)
	t.Setenv("PATH", bin)
	writeScript(t, root, "scripts/ci/env-live-key-gate.sh", "exit 0")
	got := runGitleaks(root, 30, false, false)
	if got.Passed || got.Skipped || !strings.Contains(got.Output, "gate error") {
		t.Fatalf("missing bash must be a gate error: %+v", got)
	}
}

func fakeGitleaks(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gitleaks"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
}

func TestSecretsEnvLiveKeyGateInvalidScript(t *testing.T) {
	for _, kind := range []string{"directory", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			fakeGitleaks(t)
			path := filepath.Join(root, "scripts", "ci", "env-live-key-gate.sh")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			if kind == "directory" {
				err = os.Mkdir(path, 0o755)
			} else {
				err = os.Symlink("missing.sh", path)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := runGitleaks(root, 30, false, false)
			if got.Passed || got.Skipped || !strings.Contains(got.Output, "gate error") {
				t.Fatalf("invalid env gate script must fail closed: %+v", got)
			}
		})
	}
}
