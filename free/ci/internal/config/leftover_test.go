package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLeftoverCIYaml(t *testing.T) {
	root := t.TempDir()
	if err := CheckLeftover(root); err != nil {
		t.Fatalf("clean repo: %v", err)
	}
	put := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("plugin: p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(".nself/ci.yaml")      // the new file is not a leftover
	put(".git/hooks/.ci.yaml") // .git is never searched
	put("a/b/c/d/e/.ci.yaml")  // deeper than the legacy runner looked
	put("free/plugin-gauth/.ci.yaml")
	put(".ci.yaml")
	err := CheckLeftover(root)
	e, ok := err.(*Error)
	if !ok || e.Code != CodeLeftover {
		t.Fatalf("want E621, got %v", err)
	}
	if len(e.Problems) != 2 || e.Problems[0].File != ".ci.yaml" || e.Problems[1].File != "free/plugin-gauth/.ci.yaml" {
		t.Errorf("problems: %+v", e.Problems)
	}
	if !strings.Contains(e.Error(), "nself ci config migrate --write") {
		t.Errorf("E621 does not name the migration command: %s", e)
	}
	found, err := Discover(root)
	if err != nil || len(found) != 2 {
		t.Errorf("Discover: %v %v", found, err)
	}
	if _, err := Discover(filepath.Join(root, "missing")); err == nil {
		t.Error("a missing root must be an error")
	}
}
