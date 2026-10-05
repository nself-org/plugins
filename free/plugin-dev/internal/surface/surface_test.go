package surface

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddedMatchesCapture fails when the embedded help drifts from the capture of core.
func TestEmbeddedMatchesCapture(t *testing.T) {
	for _, n := range Names {
		want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "core-surface", n+".help.txt"))
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		got, ok := Raw(n)
		if !ok || got != string(want) {
			t.Errorf("%s: embedded help differs from testdata/core-surface", n)
		}
		if len(want) == 0 {
			t.Errorf("%s: empty capture", n)
		}
	}
}

// TestRewriteOnlyAuthorPaths maps the author commands and leaves other plugin verbs alone.
func TestRewriteOnlyAuthorPaths(t *testing.T) {
	in := "nself plugin init x\nnself plugin test y\nnself plugin install z\nnself plugin remove z\nnself plugin link p"
	got := Rewrite(in)
	for _, w := range []string{"nself plugin-dev init x", "nself plugin-dev test y", "nself plugin install z", "nself plugin remove z", "nself plugin-dev link p"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in %q", w, got)
		}
	}
}

// TestHelpUnknown returns false for a name with no capture.
func TestHelpUnknown(t *testing.T) {
	if _, ok := Help("nope"); ok {
		t.Fatal("unexpected help")
	}
}
