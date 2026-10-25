//go:build unix

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO at the config path must be refused without opening it (open would block).
func TestReadRegularRefusesFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "ci.yaml")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readRegular(fifo); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO read succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readRegular blocked on a FIFO")
	}
}

// A FIFO as the migration destination must not block Diff or Write.
func TestMigrateDestinationFIFO(t *testing.T) {
	root := copyFixture(t, "legacy/gauth")
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, FileName), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Migrate(root)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := m.Diff(root); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Diff accepted a FIFO")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Diff blocked on a FIFO")
	}
}

// A regular file swapped for a FIFO after the path check must be refused on
// the open descriptor, not hang the open.
func TestReadRegularFIFOSwappedAfterCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ci.yaml")
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testHookBeforeOpen = func(p string) {
		testHookBeforeOpen = func(string) {}
		if err := os.Remove(p); err != nil {
			t.Error(err)
		}
		if err := syscall.Mkfifo(p, 0o644); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { testHookBeforeOpen = func(string) {} })
	done := make(chan error, 1)
	go func() { _, err := readRegular(path); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO swapped in after the check was read")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("readRegular blocked on a FIFO swapped in after the check")
	}
}
