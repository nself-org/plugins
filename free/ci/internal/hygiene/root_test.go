package hygiene

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJobRootIs0700AndFresh(t *testing.T) {
	base := filepath.Join(t.TempDir(), "work")
	root, err := JobRoot(base, "a1")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm() != 0o700 || !info.IsDir() {
		t.Fatalf("root %v %v", info, err)
	}
	if _, err := JobRoot(base, "a1"); codeOf(err) != CodeLeak {
		t.Fatalf("a second root for the same attempt must be refused, got %v", err)
	}
}

func TestJobRootRefusesUnsafeInput(t *testing.T) {
	base := filepath.Join(t.TempDir(), "work")
	for _, id := range []string{"", ".", "..", "a/b", "../x", "-x", "a b", "a\x00b"} {
		if _, err := JobRoot(base, id); codeOf(err) != CodeLeak {
			t.Errorf("attempt %q accepted: %v", id, err)
		}
	}
	if _, err := JobRoot("relative/work", "a1"); err == nil {
		t.Error("relative base accepted")
	}
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := JobRoot(link, "a1"); err == nil {
		t.Error("symlinked base accepted")
	}
	if entries, _ := os.ReadDir(real); len(entries) != 0 {
		t.Errorf("a root was created through the symlink: %v", entries)
	}
}
