package main

// Purpose: TestConsentNoPrompt (P7-CI-62, CI D31): every remote or destructive command of commands.yaml runs with
//          NSELF_NONINTERACTIVE=1 on a fixture store, the way a machine caller starts it after its consent, and must
//          neither prompt nor hang (10 s).
// Inputs:  the spec rows; a fixture HOME, state home and repository; stub ssh/scp/gh/docker/curl that fail at once.
// Constraints: no network (PATH holds only the stubs), nothing outside t.TempDir() is written; stdin is a pipe nobody
//          writes to, so a read of it blocks and is reported as a prompt; a prompt marker in the output also fails.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const consentBound = 10 * time.Second

var promptMarkers = []string{"[y/n]", "(y/n)", "type yes", "password:", "passphrase:", "press enter", "continue?"}

// consentArgs are the positional values a fixture run needs; every other positional is "nosuch".
var consentArgs = map[string][]string{
	"artifacts":    {"prune", "--dry-run"},
	"runs resolve": {"nosuch", "--idempotency-key", "k", "--abandon"},
	"runs prune":   {"--dry-run"},
}

// runBounded starts name with args, a stdin nobody writes to and combined output; hung is true when it outlives the
// bound (it is then killed). A command that reads stdin therefore shows up as hung.
func runBounded(t *testing.T, bound time.Duration, name string, args ...string) (out string, hung bool) {
	t.Helper()
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "NSELF_CI_AS_BINARY=1")
	cmd.Stdin = stdinR
	cmd.WaitDelay = time.Second
	b, _ := cmd.CombinedOutput()
	return string(b), ctx.Err() != nil
}

// promptIn returns the prompt marker found in out, or "".
func promptIn(out string) string {
	low := strings.ToLower(out)
	for _, m := range promptMarkers {
		if strings.Contains(low, m) {
			return m
		}
	}
	return ""
}

// TestConsentDetectorCatchesPromptAndHang: the harness fails a command that reads stdin or prints a question.
func TestConsentDetectorCatchesPromptAndHang(t *testing.T) {
	_, hung := runBounded(t, 300*time.Millisecond, "/bin/sh", "-c", "read answer")
	if !hung {
		t.Fatal("a command blocked on stdin was not reported")
	}
	out, hung := runBounded(t, consentBound, "/bin/sh", "-c", "echo 'Delete everything? [y/N]' >&2; exit 1")
	if hung || promptIn(out) == "" {
		t.Fatalf("a printed prompt was not reported: hung=%v out=%q", hung, out)
	}
}

func TestConsentNoPrompt(t *testing.T) {
	s := loadSpec(t)
	for _, p := range consentProblems(s) {
		t.Error(p)
	}
	stubs := t.TempDir()
	for _, name := range []string{"ssh", "scp", "gh", "docker", "curl", "git"} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/sh\nexit 255\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	home, state, repo := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("PATH", stubs)
	t.Setenv("HOME", home)
	t.Setenv("NSELF_CI_HOME", state)
	t.Setenv("NSELF_CI_REPO", repo)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_NONINTERACTIVE", "1")
	t.Setenv("NSELF_INVOKED_BY", "mcp")
	t.Setenv("NSELF_CLI_VERSION", "1.5.0")
	rows := append([]specRow{}, s.Commands...)
	ran := 0
	for _, r := range rows {
		if r.SideEffect != "remote" && r.SideEffect != "destructive" {
			continue
		}
		ran++
		argv := strings.Fields(r.Path)
		if a, ok := consentArgs[r.Path]; ok {
			argv = append(argv, a...)
		} else {
			for _, a := range r.Args {
				if !a.Variadic {
					argv = append(argv, "nosuch")
				}
			}
		}
		if r.Confirm != nil {
			for _, f := range r.Confirm.Flags {
				argv = append(argv, "--"+f)
			}
		}
		t.Run(r.Path, func(t *testing.T) {
			out, hung := runBounded(t, consentBound, os.Args[0], argv...)
			if hung {
				t.Fatalf("`nself ci %s` did not finish in %s (a prompt or a hang)", strings.Join(argv, " "), consentBound)
			}
			if strings.TrimSpace(out) == "" {
				t.Errorf("`nself ci %s` printed nothing: the command did not run", strings.Join(argv, " "))
			}
			if m := promptIn(out); m != "" {
				t.Errorf("`nself ci %s` prompts (%q): %.200s", strings.Join(argv, " "), m, out)
			}
		})
	}
	if ran < 25 {
		t.Fatalf("only %d remote or destructive rows ran", ran)
	}
}
