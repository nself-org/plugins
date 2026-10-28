//go:build unix

package enroll

// Purpose: the agent state dir checks (D-1259): a symlinked pki directory is refused and its target keeps
// its mode, a state dir writable by others is refused, and a normal state dir still works.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnrollSaveRefusesSymlinkedPKIDir(t *testing.T) {
	ca := newCA(t, nil)
	res := joined(t, ca, "node-sl", 0)
	state := filepath.Join(t.TempDir(), "agent")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, dir(state)); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(state, res, time.Unix(1_800_000_000, 0)); err == nil {
		t.Fatal("a symlinked pki directory was accepted")
	}
	if fi, err := os.Stat(target); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("the chmod followed the link: %v %v", fi.Mode().Perm(), err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("credential files were written through the link: %v", entries)
	}
}

func TestEnrollSaveRefusesStateWritableByOthers(t *testing.T) {
	ca := newCA(t, nil)
	res := joined(t, ca, "node-w", 0)
	state := filepath.Join(t.TempDir(), "agent")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(state, res, time.Unix(1_800_000_000, 0)); err == nil {
		t.Fatal("a world-writable state dir was accepted")
	}
	if err := os.Chmod(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveCredential(state, res, time.Unix(1_800_000_000, 0)); err != nil {
		t.Fatalf("a private state dir: %v", err)
	}
}
