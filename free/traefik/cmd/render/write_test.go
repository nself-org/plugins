package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// withUmask runs the test body under mask and restores the old umask after.
func withUmask(t *testing.T, mask int) {
	t.Helper()
	old := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(old) })
}

// With umask 022 the temp file is born 0644, so chmod is never called and an EPERM-only
// filesystem (Colima virtiofs) still gets a 0644 file.
func TestWriteAtomicSkipsChmodWhenModeIsRight(t *testing.T) {
	withUmask(t, 0o022)
	calls := 0
	old := chmodFile
	chmodFile = func(*os.File, os.FileMode) error { calls++; return syscall.EPERM }
	t.Cleanup(func() { chmodFile = old })
	dir := t.TempDir()
	if err := writeAtomic(dir, "dynamic.yml", []byte("x")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "dynamic.yml"))
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("stat %v, err %v", fi, err)
	}
	if calls != 0 {
		t.Fatalf("chmod called %d times", calls)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Fatalf("leftovers: %v", ents)
	}
}

// With umask 077 the file is born 0600: chmod runs, and its failure fails the write with no
// temp file and no target.
func TestWriteAtomicChmodFailureLeavesNothing(t *testing.T) {
	withUmask(t, 0o077)
	calls := 0
	old := chmodFile
	chmodFile = func(*os.File, os.FileMode) error { calls++; return syscall.EPERM }
	t.Cleanup(func() { chmodFile = old })
	dir := t.TempDir()
	if err := writeAtomic(dir, "dynamic.yml", []byte("x")); err == nil {
		t.Fatal("want the chmod error")
	}
	if calls != 1 {
		t.Fatalf("chmod called %d times, want 1", calls)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("leftovers: %v", ents)
	}
}

// With umask 077 and a working chmod the result is still 0644.
func TestWriteAtomicWidensNarrowedMode(t *testing.T) {
	withUmask(t, 0o077)
	dir := t.TempDir()
	if err := writeAtomic(dir, "f", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "f")); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", fi.Mode())
	}
}
