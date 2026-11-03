package specs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func treeRestore(t *testing.T, e *env, id string) *Session {
	t.Helper()
	e.running(id, ownerFacts)
	s := e.begin(e.input(id, ownerFacts))
	if got := strings.Join(resultsOf(e.uses(id)), ","); got != "go:hit" {
		t.Fatalf("restore %s: %s", id, got)
	}
	for name, want := range map[string]string{"mod/example.com/m@v1/m.go": "package m", "build/ab/cd-d": "compiled"} {
		parts := strings.SplitN(name, "/", 2)
		if got := getFile(t, filepath.Join(s.dir(parts[0]), parts[1])); got != want {
			t.Fatalf("restore %s %s: %q, want %q", id, name, got, want)
		}
	}
	return s
}

func TestCloneTreeFallback(t *testing.T) {
	e := newEnv(t)
	saved(t, e, "fallback-save", ownerFacts).Cleanup()
	readOnly := filepath.Join(t.TempDir(), "read-only")
	if err := os.Mkdir(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
	e.eng.Trees = filepath.Join(readOnly, "trees")
	// Root may bypass directory permissions. Keep the cache unavailable in
	// that case too, with a regular file at the requested directory path.
	if err := os.Mkdir(e.eng.Trees, 0o700); err == nil {
		if err := os.Remove(e.eng.Trees); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(e.eng.Trees, nil, 0o400); err != nil {
			t.Fatal(err)
		}
	} else if !os.IsPermission(err) {
		t.Fatalf("probe read-only directory: %v", err)
	}
	treeRestore(t, e, "fallback-hit").Cleanup()
}
