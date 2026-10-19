package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A tracked relative symlink that stays inside the source (the plugins repo's
// AGENTS.md -> .claude/CLAUDE.md) is copied as a symlink; one that escapes the
// source is refused. Regression test for D-0999.
func TestCopyTreeSymlinks(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		wantErr string
	}{
		{"relative inside", ".claude/CLAUDE.md", ""},
		{"dangling inside", ".claude/missing.md", ""},
		{"parent escape", "../outside.txt", "symlink in job source"},
		{"nested escape", ".claude/../../outside.txt", "symlink in job source"},
		{"absolute", "/etc/hosts", "symlink in job source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, dst := t.TempDir(), filepath.Join(t.TempDir(), "ws")
			if err := os.Mkdir(dst, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(src, ".claude"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(src, ".claude", "CLAUDE.md"), []byte("rules"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(tc.target, filepath.Join(src, "AGENTS.md")); err != nil {
				t.Fatal(err)
			}
			err := copyTree(src, dst, "")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("copyTree error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("copyTree: %v", err)
			}
			got, err := os.Readlink(filepath.Join(dst, "AGENTS.md"))
			if err != nil || got != tc.target {
				t.Fatalf("Readlink = %q, %v; want %q", got, err, tc.target)
			}
			if tc.name == "relative inside" {
				b, err := os.ReadFile(filepath.Join(dst, "AGENTS.md"))
				if err != nil || string(b) != "rules" {
					t.Fatalf("read through copied link = %q, %v", b, err)
				}
			}
		})
	}
}

// Links are vetted one by one, so chains, links to directories and ".." after a
// name are the cases that could still escape. Each case lists tracked links
// (path -> target); the copy must either fail or leave every link resolving
// inside the destination.
func TestCopyTreeSymlinkChains(t *testing.T) {
	cases := []struct {
		name   string
		links  map[string]string
		refuse bool
	}{
		{"chain inside", map[string]string{"a": "b", "b": "sub/f.txt"}, false},
		{"chain ends outside", map[string]string{"a": "b", "b": "../out"}, true},
		{"dir link up then descend", map[string]string{"sub/up": "..", "sub/l": "up/sub/f.txt"}, false},
		{"dot dot after dir link escapes", map[string]string{"sub/up": "..", "sub/l": "up/../.."}, true},
		{"dot dot after name even if it cleans inside", map[string]string{"l": "sub/../sub/f.txt"}, true},
		{"deep link climbs to root", map[string]string{"sub/deep/l": "../../top.txt"}, false},
		{"deep link climbs past root", map[string]string{"sub/deep/l": "../../../top.txt"}, true},
		{"link to root", map[string]string{"sub/l": ".."}, false},
		{"empty target", map[string]string{"l": ""}, true},
		{"loop", map[string]string{"x": "y", "y": "x"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, dst := t.TempDir(), filepath.Join(t.TempDir(), "ws")
			if err := os.Mkdir(dst, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(src, "sub", "deep"), 0700); err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{"top.txt", "sub/f.txt"} {
				if err := os.WriteFile(filepath.Join(src, f), []byte("x"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for path, target := range tc.links {
				if err := os.Symlink(target, filepath.Join(src, path)); err != nil {
					t.Skipf("cannot create link %q -> %q: %v", path, target, err)
				}
			}
			err := copyTree(src, dst, "")
			if tc.refuse {
				if err == nil || !strings.Contains(err.Error(), "symlink in job source") {
					t.Fatalf("copyTree error = %v, want refusal", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("copyTree: %v", err)
			}
			root, err := filepath.EvalSymlinks(dst)
			if err != nil {
				t.Fatal(err)
			}
			for path := range tc.links {
				got, err := filepath.EvalSymlinks(filepath.Join(dst, path))
				if err != nil {
					continue // dangling or looping: nothing reachable
				}
				if got != root && !strings.HasPrefix(got, root+string(filepath.Separator)) {
					t.Fatalf("link %s resolves to %s, outside %s", path, got, root)
				}
			}
		})
	}
}
