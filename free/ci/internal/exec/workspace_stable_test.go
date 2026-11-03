package exec

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStableWorkspaceSamePathAcrossAttempts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows always falls back to attempt workspaces")
	}
	home := t.TempDir()
	first, release, err := acquireWorkspace(home, "attempt-one", "root\x00job\x00trusted")
	if err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(first, "leftover")
	if err := os.WriteFile(leftover, []byte("old attempt"), 0600); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	second, releaseSecond, err := acquireWorkspace(home, "attempt-two", "root\x00job\x00trusted")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if first != second || filepath.Dir(second) != filepath.Join(home, "ws") {
		t.Fatalf("workspace paths: first %q, second %q", first, second)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover survived: %v", err)
	}
}

func TestStableWorkspaceBusyFallsBack(t *testing.T) {
	home := t.TempDir()
	_, release, err := acquireWorkspace(home, "attempt-one", "root\x00job\x00trusted")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	dir, releaseSecond, err := acquireWorkspace(home, "attempt-two", "root\x00job\x00trusted")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if want := filepath.Join(home, "work", "attempt-two"); dir != want {
		t.Fatalf("busy workspace = %q, want %q", dir, want)
	}
}

func TestStableWorkspaceKeySeparatesTrust(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows always falls back to attempt workspaces")
	}
	home := t.TempDir()
	first, release, err := acquireWorkspace(home, "attempt", "root\x00job\x00trusted")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	second, releaseSecond, err := acquireWorkspace(home, "attempt", "root\x00job\x00untrusted")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	if first == second || filepath.Dir(first) != filepath.Join(home, "ws") || filepath.Dir(second) != filepath.Join(home, "ws") {
		t.Fatalf("trust classes must select distinct stable paths: %q, %q", first, second)
	}
}

func TestEmptyKeyKeepsAttemptPath(t *testing.T) {
	home := t.TempDir()
	dir, release, err := acquireWorkspace(home, "attempt", "")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if want := filepath.Join(home, "work", "attempt"); dir != want {
		t.Fatalf("empty-key workspace = %q, want %q", dir, want)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("attempt workspace not created: %v", err)
	}
}
