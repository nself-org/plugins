package publish

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string) {
	t.Helper()
	for name, body := range map[string]string{"a.txt": "a", "sub/b.txt": "b", "sub/z.txt": "z"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCopyTreeNeverFollowsADestinationSymlink(t *testing.T) {
	src, elsewhere, base := t.TempDir(), t.TempDir(), t.TempDir()
	writeTree(t, src)
	link := filepath.Join(base, "dest")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := copyTree(src, link); err == nil {
		t.Fatal("a symlink destination was accepted")
	}
	if ents, _ := os.ReadDir(elsewhere); len(ents) != 0 {
		t.Fatalf("files were written through the symlink: %v", ents)
	}
	// a symlink planted inside an existing empty destination, where a directory is to be made
	dst := filepath.Join(base, "real")
	if err := os.Mkdir(dst, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(dst, "sub")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := copyTree(src, dst); err == nil {
		t.Fatal("a planted directory symlink was followed")
	}
	if ents, _ := os.ReadDir(elsewhere); len(ents) != 0 {
		t.Fatalf("files were written through the planted symlink: %v", ents)
	}
	if _, err := os.Lstat(filepath.Join(dst, "a.txt")); err == nil {
		t.Fatal("a failed copy left a file behind")
	}
}

func TestCopyTreeCreatesAndCleansUp(t *testing.T) {
	src, base := t.TempDir(), t.TempDir()
	writeTree(t, src)
	dst := filepath.Join(base, "out")
	if _, _, err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "sub", "b.txt")); err != nil || string(b) != "b" {
		t.Fatalf("copy: %q %v", b, err)
	}
	if info, _ := os.Stat(filepath.Join(dst, "a.txt")); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	// a source with an irregular entry fails and removes everything it made, the new destination included
	if err := os.Symlink("/etc/hosts", filepath.Join(src, "sub", "zz-link")); err != nil {
		t.Fatal(err)
	}
	dst2 := filepath.Join(base, "out2")
	if _, _, err := copyTree(src, dst2); err == nil {
		t.Fatal("a symlink in the source was copied")
	}
	if _, err := os.Lstat(dst2); err == nil {
		t.Fatal("a failed copy left the new destination behind")
	}
	// an existing empty destination stays, emptied
	dst3 := filepath.Join(base, "out3")
	if err := os.Mkdir(dst3, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := copyTree(src, dst3); err == nil {
		t.Fatal("expected the same failure")
	}
	if ents, _ := os.ReadDir(dst3); len(ents) != 0 {
		t.Fatalf("leftovers in a pre-existing destination: %v", ents)
	}
}

// The rollback backup of a release older than the sweep age is live while the
// ledger write is pending: another run's sweep must not delete it, and undo
// must put the original back.
func TestSweepNeverTakesALiveRollbackBackup(t *testing.T) {
	home := t.TempDir()
	releases := releasesDir(home)
	target := filepath.Join(releases, "run-1")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "SHA256SUMS"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	aged := time.Now().Add(-5 * time.Hour)
	if err := os.Chtimes(target, aged, aged); err != nil {
		t.Fatal(err)
	}
	staged, err := stage(home) // creates the staging directory for run-1's rebuild
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(staged, "SHA256SUMS"), []byte("rebuilt"), 0o600); err != nil {
		t.Fatal(err)
	}
	commit, undo, err := installDir(staged, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stage(home); err != nil { // another run starts and sweeps
		t.Fatal(err)
	}
	undo() // the ledger write failed
	if b, err := os.ReadFile(filepath.Join(target, "SHA256SUMS")); err != nil || string(b) != "original" {
		t.Fatalf("the original release was not restored: %q %v", b, err)
	}
	_ = commit
}

// A destination renamed away and replaced by a symlink to a directory holding
// same-named files must not make undo delete those files: undo works through
// the handle opened at copy time.
func TestCopyTreeUndoNeverFollowsAReplacedDestination(t *testing.T) {
	src, base, victim := t.TempDir(), t.TempDir(), t.TempDir()
	writeTree(t, src)
	for _, name := range []string{"a.txt", "sub"} {
		if err := os.WriteFile(filepath.Join(victim, name+".keep"), []byte("v"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(victim, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "sub/b.txt", "sub/z.txt"} {
		if err := os.WriteFile(filepath.Join(victim, filepath.FromSlash(name)), []byte("victim"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dst := filepath.Join(base, "out")
	undo, done, err := copyTree(src, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	moved := filepath.Join(base, "moved")
	if err = os.Rename(dst, moved); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(victim, dst); err != nil {
		t.Fatal(err)
	}
	undo()
	for _, name := range []string{"a.txt", "sub/b.txt", "sub/z.txt"} {
		if b, rerr := os.ReadFile(filepath.Join(victim, filepath.FromSlash(name))); rerr != nil || string(b) != "victim" {
			t.Fatalf("undo reached through the replaced destination: %s %q %v", name, b, rerr)
		}
	}
	if ents, _ := os.ReadDir(moved); len(ents) != 0 {
		t.Fatalf("the copied files were not removed from the original directory: %v", ents)
	}
}
