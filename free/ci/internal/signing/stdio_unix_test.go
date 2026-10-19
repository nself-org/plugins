//go:build unix

package signing_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func runner(t *testing.T) *signing.FunnelRunner { return &signing.FunnelRunner{Home: t.TempDir()} }

func TestFunnelRunnerFeedsStdin(t *testing.T) {
	r := runner(t)
	payload := strings.Repeat("payload-line\n", 20000) // larger than a pipe buffer
	out, code, err := r.RunStdio(context.Background(), signing.StdioCall{Argv: []string{"cat"}, Stdin: []byte(payload), Timeout: 20 * time.Second})
	if err != nil || code != 0 || string(out) != payload {
		t.Fatalf("code %d err %v, %d bytes back", code, err, len(out))
	}
}

func TestFunnelRunnerNilStdinIsDevNull(t *testing.T) {
	out, code, err := runner(t).RunStdio(context.Background(), signing.StdioCall{Argv: []string{"cat"}, Timeout: 10 * time.Second})
	if err != nil || code != 0 || len(out) != 0 {
		t.Fatalf("%d %v %q", code, err, out)
	}
}

func TestFunnelRunnerExitCodeEnvAndDir(t *testing.T) {
	r := runner(t)
	dir := t.TempDir()
	out, code, err := r.RunStdio(context.Background(), signing.StdioCall{Argv: []string{"/bin/sh", "-c", `pwd; echo "$MARK"; exit 7`}, Env: []string{"MARK=hello"}, Dir: dir, Timeout: 10 * time.Second})
	want, _ := filepath.EvalSymlinks(dir)
	got := strings.Fields(string(out))
	if err != nil || code != 7 || len(got) != 2 || got[1] != "hello" {
		t.Fatalf("code %d err %v out %q", code, err, out)
	}
	if real, _ := filepath.EvalSymlinks(got[0]); real != want {
		t.Fatalf("ran in %s, want %s", got[0], want)
	}
	if _, code, _ := r.RunStdio(context.Background(), signing.StdioCall{Argv: []string{"true"}, Dir: filepath.Join(dir, "missing"), Timeout: 10 * time.Second}); code != 126 {
		t.Fatalf("a missing directory exited %d", code)
	}
}

// A child that never reads its stdin must not leave the writer blocked.
func TestFunnelRunnerChildNeverReadsStdin(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, code, err := runner(t).RunStdio(context.Background(), signing.StdioCall{Argv: []string{"/nonexistent-program"}, Stdin: []byte("x"), Timeout: 10 * time.Second})
		if err != nil || code == 0 {
			t.Errorf("%d %v", code, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("RunStdio hung on a child that never opened its stdin")
	}
}

func TestFunnelRunnerTimeoutAndEmptyArgv(t *testing.T) {
	if _, _, err := runner(t).RunStdio(context.Background(), signing.StdioCall{Argv: []string{"sleep", "30"}, Timeout: 300 * time.Millisecond}); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("a timed-out call: %v", err)
	}
	if _, _, err := runner(t).RunStdio(context.Background(), signing.StdioCall{}); !signing.IsReason(err, signing.ReasonSignerFailed) {
		t.Fatalf("an empty call: %v", err)
	}
}

func TestFunnelRunnerKeepsTheOperatorEnvironmentOut(t *testing.T) {
	t.Setenv("SOME_OPERATOR_TOKEN", "must-not-leak")
	out, _, err := runner(t).RunStdio(context.Background(), signing.StdioCall{Argv: []string{"/usr/bin/env"}, Timeout: 10 * time.Second})
	if err != nil || strings.Contains(string(out), "must-not-leak") || !strings.Contains(string(out), "PATH=") {
		t.Fatalf("%v %q", err, out)
	}
	_ = os.Getenv
}

// D-1095: stdin travels on the funnel's own Stdin, so no FIFO, private
// directory or NSELF_CI_STDIN variable is involved (an unusable TMPDIR is fine).
func TestFunnelRunnerStdinNeedsNoFifoOrTempDirectory(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	out, code, err := runner(t).RunStdio(context.Background(), signing.StdioCall{Argv: []string{"/bin/sh", "-c", `echo "[${NSELF_CI_STDIN}]"; cat`}, Stdin: []byte("key-bytes"), Timeout: 10 * time.Second})
	if err != nil || code != 0 || string(out) != "[]\nkey-bytes" {
		t.Fatalf("%d %v %q", code, err, out)
	}
}
