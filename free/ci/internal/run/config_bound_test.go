package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configGit answers the two questions ConfigOutsideTree asks of git.
func configGit(tree, hash string, exit int) Capture {
	return func(_ context.Context, argv ...string) (string, int, error) {
		for _, a := range argv {
			switch a {
			case "ls-tree":
				return tree, exit, nil
			case "hash-object":
				return hash + "\n", 0, nil
			}
		}
		return "", 1, nil
	}
}

func TestConfigOutsideTree(t *testing.T) {
	const rel = ".nself/ci.yaml"
	blob := strings.Repeat("ab", 20)
	tracked := "100644 blob " + blob + "\t" + rel + "\n"
	write := func(root string) string {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("version: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, tc := range []struct {
		name  string
		setup func(root string) (path string)
		git   Capture
		want  bool
	}{
		{"tracked regular file", write, configGit(tracked, blob, 0), false},
		{"tracked executable blob", write, configGit("100755 blob "+blob+"\t"+rel, blob, 0), false},
		{"no config file at all", func(r string) string { return filepath.Join(r, rel) }, configGit("", "", 0), false},
		{"ignored or untracked: HEAD has no such path", write, configGit("", blob, 0), true},
		{"git cannot answer", write, configGit("", blob, 128), true},
		{"edited since HEAD (status hidden by assume-unchanged)", write, configGit(tracked, strings.Repeat("cd", 20), 0), true},
		{"HEAD holds a symlink", write, configGit("120000 blob "+blob+"\t"+rel, blob, 0), true},
		{"HEAD holds a submodule entry", write, configGit("160000 commit "+blob+"\t"+rel, blob, 0), true},
		{"another path with --config", func(r string) string { write(r); return filepath.Join(r, "other.yaml") }, configGit(tracked, blob, 0), true},
		{"config is a symlink to a file outside the tree", func(r string) string {
			out := filepath.Join(t.TempDir(), "ci.yaml")
			if err := os.WriteFile(out, []byte("version: 1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(r, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(out, p); err != nil {
				t.Skip("no symlinks here")
			}
			return p
		}, configGit(tracked, blob, 0), true},
		{"the .nself directory is a symlink", func(r string) string {
			real := filepath.Join(t.TempDir(), "dot-nself")
			if err := os.MkdirAll(real, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(real, "ci.yaml"), []byte("version: 1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(real, filepath.Join(r, ".nself")); err != nil {
				t.Skip("no symlinks here")
			}
			return filepath.Join(r, rel)
		}, configGit(tracked, blob, 0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := tc.setup(root)
			if got := ConfigOutsideTree(context.Background(), root, path, rel, tc.git); got != tc.want {
				t.Fatalf("ConfigOutsideTree = %v, want %v", got, tc.want)
			}
		})
	}
}
