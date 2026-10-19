package specs

// Purpose: regression tests for D-1056: the tool-home reads of credentialsHidden
// and seed are bounded and never block, whatever a job left in the tool home.
// Inputs:  a temporary CARGO_HOME holding hostile files (FIFO, device symlink).
// Outputs: test results only.
// Constraints: no network; the FIFO test must fail rather than hang.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func cargoSession(home string) *Session {
	return &Session{in: BeginInput{Getenv: func(k string) string {
		if k == "CARGO_HOME" {
			return home
		}
		return ""
	}}}
}

// hiddenWithin runs credentialsHidden and fails the test if it does not return in time.
func hiddenWithin(t *testing.T, home string) bool {
	t.Helper()
	done := make(chan bool, 1)
	go func() { done <- cargoSession(home).credentialsHidden(cargoSpec(t)) }()
	select {
	case got := <-done:
		return got
	case <-time.After(3 * time.Second):
		// unblock the stuck reader so the goroutine ends, then fail
		if f, err := os.OpenFile(filepath.Join(home, "config.toml"), os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
		t.Fatal("credentialsHidden blocked on a FIFO config")
		return false
	}
}

func cargoSpec(t *testing.T) Spec {
	t.Helper()
	sp, ok := Get("cargo")
	if !ok {
		t.Fatal("no cargo spec")
	}
	return sp
}

func TestCredentialsHiddenFIFOConfigDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(home, "config.toml"), 0o600); err != nil {
		t.Skipf("no FIFO support: %v", err)
	}
	if !hiddenWithin(t, home) {
		t.Fatal("an unreadable (non-regular) config must disable the rebind, not be treated as safe")
	}
}

func TestCredentialsHiddenDeviceSymlinkIsBounded(t *testing.T) {
	if _, err := os.Stat("/dev/zero"); err != nil {
		t.Skip("no /dev/zero")
	}
	home := t.TempDir()
	if err := os.Symlink("/dev/zero", filepath.Join(home, "config.toml")); err != nil {
		t.Fatal(err)
	}
	if !hiddenWithin(t, home) {
		t.Fatal("a device behind the config name must disable the rebind")
	}
}

func TestCredentialsHiddenOversizedConfigFailsClosed(t *testing.T) {
	home := t.TempDir()
	putFile(t, filepath.Join(home, "config.toml"), strings.Repeat("#", 2<<20))
	if !hiddenWithin(t, home) {
		t.Fatal("a config over the 1 MiB bound must disable the rebind")
	}
}

func TestCredentialsHiddenKeepsRegularBehaviour(t *testing.T) {
	home := t.TempDir()
	putFile(t, filepath.Join(home, "config.toml"), "[build]\njobs = 2\n")
	if hiddenWithin(t, home) {
		t.Fatal("a plain config is not credentials")
	}
	putFile(t, filepath.Join(home, "config.toml"), "[registries.corp]\nindex = \"x\"\n")
	if !hiddenWithin(t, home) {
		t.Fatal("a config naming a registry is credentials")
	}
}

func TestSeedFIFOConfigDoesNotBlock(t *testing.T) {
	home := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(home, "config.toml"), 0o600); err != nil {
		t.Skipf("no FIFO support: %v", err)
	}
	base, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	s := cargoSession(home)
	s.base = base
	it := &Item{Spec: cargoSpec(t)}
	done := make(chan struct{})
	go func() { s.seed(it); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if f, err := os.OpenFile(filepath.Join(home, "config.toml"), os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
		t.Fatal("seed blocked on a FIFO config")
	}
}
