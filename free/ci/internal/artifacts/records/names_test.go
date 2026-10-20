package records

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Names gives the record names the collector will commit.
func TestNamesMatchesTheCollectorNaming(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"dist/a.tar.gz", "dist/b.tar.gz", "bin/tool"} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	got, err := Names(context.Background(), root, []Decl{{Path: "dist/*.tar.gz", Name: "pkg"}, {Path: "bin/tool", Name: "tool"}, {Path: "bin/tool"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pkg/a.tar.gz", "pkg/b.tar.gz", "tool", "bin/tool"}
	if len(got) != len(want) {
		t.Fatalf("%d names, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("name %d = %q, want %q", i, got[i].Name, w)
		}
	}
	if _, err = Names(context.Background(), root, []Decl{{Path: "missing/*"}}); err == nil {
		t.Error("a declaration that selects nothing was accepted")
	}
}
