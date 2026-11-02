//go:build unix

package hygiene

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

// fakeDockerOnPath puts a docker script on PATH that appends each invocation to a log and exits with
// listCode for listings and infoCode for `info`. It returns the log path.
func fakeDockerOnPath(t *testing.T, listCode, infoCode string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\ncase \"$1\" in info) exit " + infoCode + ";; esac\nexit " + listCode + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func readCalls(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// TestListSkipsInfoWhenTheListingAnswers: a listing that succeeds proves the daemon answers, so
// List must not pay for `info` first (P7-CI-46: podman info costs ~690 ms on hosted runners).
func TestListSkipsInfoWhenTheListingAnswers(t *testing.T) {
	log := fakeDockerOnPath(t, "0", "0")
	r := &ExecRuntime{Binaries: []string{"docker"}}
	if _, err := r.List(context.Background(), exec.KindContainer, ""); err != nil {
		t.Fatal(err)
	}
	got := readCalls(t, log)
	if !strings.Contains(got, "ps ") {
		t.Fatalf("the listing never ran: %q", got)
	}
	if strings.Contains(got, "info") {
		t.Fatalf("List probed info although the listing answered: %q", got)
	}
}

// TestListReportsADownRuntimeWhenListingAndInfoFail: a failed listing still falls back to the
// info probe, and a failed probe is reported as a RuntimeDownError, as before.
func TestListReportsADownRuntimeWhenListingAndInfoFail(t *testing.T) {
	log := fakeDockerOnPath(t, "1", "1")
	r := &ExecRuntime{Binaries: []string{"docker"}}
	_, err := r.List(context.Background(), exec.KindContainer, "")
	var down *RuntimeDownError
	if err == nil || !errors.As(err, &down) {
		t.Fatalf("List = %v, want a RuntimeDownError", err)
	}
	if !strings.Contains(readCalls(t, log), "info") {
		t.Fatal("a failed listing must fall back to the info probe")
	}
}

// TestListReportsNoRuntimeWhenNoneIsInstalled keeps main's answer for a host with no runtime CLI:
// the same "no container runtime is installed" error, without a listing.
func TestListReportsNoRuntimeWhenNoneIsInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	r := &ExecRuntime{Binaries: []string{"docker"}}
	got, err := r.List(context.Background(), exec.KindContainer, "")
	if err == nil || !strings.Contains(err.Error(), "no container runtime is installed") || len(got) != 0 {
		t.Fatalf("List = %v, %v; want no resources and the no-runtime error", got, err)
	}
}
