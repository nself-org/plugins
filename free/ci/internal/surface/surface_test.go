package surface

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddedHelpMatchesCapture fails when the embedded help drifts from the
// capture recorded under testdata/core-surface.
func TestEmbeddedHelpMatchesCapture(t *testing.T) {
	files := map[string]string{
		"":        "ci.help.txt",
		"build":   "ci-build.help.txt",
		"forgejo": "ci-forgejo.help.txt",
		"serve":   "ci-serve.help.txt",
	}
	for key, f := range files {
		want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "core-surface", f))
		if err != nil {
			t.Fatalf("read capture %s: %v", f, err)
		}
		got, ok := Help(key)
		if !ok {
			t.Fatalf("no embedded help for %q", key)
		}
		if got != string(want) {
			t.Errorf("embedded help for %q differs from testdata/core-surface/%s (re-run scripts/capture-core-surface.sh)", key, f)
		}
		if !strings.Contains(got, "Usage:") {
			t.Errorf("help for %q has no Usage section", key)
		}
	}
}

// TestUnknownKey returns false for a command with no captured help.
func TestUnknownKey(t *testing.T) {
	if _, ok := Help("run"); ok {
		t.Fatal("run has no captured core help")
	}
	if len(Keys()) != 4 {
		t.Fatalf("Keys = %v", Keys())
	}
}
