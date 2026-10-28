package authz_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func TestOpenArtifactGetDestPinsParentAndLeaf(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[existing], func(t *testing.T) {
			base := scratch(t)
			cwd := filepath.Join(base, "work")
			parent := filepath.Join(base, "parent")
			outside := filepath.Join(base, "outside")
			for _, p := range []string{cwd, parent, outside} {
				mustMkdir(t, p)
			}
			dest := filepath.Join(parent, "out")
			if existing {
				mustMkdir(t, dest)
			}
			root, _, err := authz.OpenArtifactGetDest(dest, cwd)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			moved := filepath.Join(base, "moved")
			if err := os.Rename(parent, moved); err != nil {
				t.Fatal(err)
			}
			mustMkdir(t, filepath.Join(outside, "out"))
			if err := os.Symlink(outside, parent); err != nil {
				t.Fatal(err)
			}
			file, err := root.OpenFile("artifact", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			file.Close()
			if _, err := os.Stat(filepath.Join(outside, "out", "artifact")); !os.IsNotExist(err) {
				t.Fatalf("write escaped to replacement parent: %v", err)
			}
			if _, err := os.Stat(filepath.Join(moved, "out", "artifact")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenArtifactGetDestRefusesNonemptyAndSymlink(t *testing.T) {
	base := scratch(t)
	cwd := filepath.Join(base, "work")
	mustMkdir(t, cwd)
	full := filepath.Join(base, "full")
	mustMkdir(t, full)
	mustWrite(t, filepath.Join(full, "artifact"))
	link := filepath.Join(base, "link")
	if err := os.Symlink(full, link); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{full, link, cwd, filepath.Join(base, ".git", "out")} {
		root, _, err := authz.OpenArtifactGetDest(dest, cwd)
		if root != nil {
			root.Close()
		}
		if err == nil {
			t.Fatalf("accepted %s", dest)
		}
	}
}
