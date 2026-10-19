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
			err := copyTree(src, dst)
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
