package publish

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := copyTree(src, link); err == nil {
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
	if err := copyTree(src, dst); err == nil {
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
	if err := copyTree(src, dst); err != nil {
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
	if err := copyTree(src, dst2); err == nil {
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
	if err := copyTree(src, dst3); err == nil {
		t.Fatal("expected the same failure")
	}
	if ents, _ := os.ReadDir(dst3); len(ents) != 0 {
		t.Fatalf("leftovers in a pre-existing destination: %v", ents)
	}
}
