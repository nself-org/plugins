package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// statusBackground matches a migrate.Status call on a context with no deadline.
var statusBackground = regexp.MustCompile(`migrate\.Status\(\s*context\.(Background|TODO)\(\)`)

// A hung database must fail /health, not hang it (D-0887): the template bounds Status with a timeout.
func TestHelperTemplateBoundsStatus(t *testing.T) {
	if statusBackground.MatchString(helperTmpl) {
		t.Fatal("helperTmpl calls migrate.Status with an unbounded context")
	}
	for _, want := range []string{`context.WithTimeout(context.Background(), 5*time.Second)`, "defer cancel()", `"time"`} {
		if !strings.Contains(helperTmpl, want) {
			t.Errorf("helperTmpl missing %q", want)
		}
	}
}

// The generated copies in the plugins and the conformance goldens carry the same fix. Every Go file is
// scanned (not just */cmd and */internal): hand-placed helpers live at other depths (D-0887).
func TestGeneratedHelpersBoundStatus(t *testing.T) {
	var files []string
	for _, root := range []string{filepath.Join("..", "..", "..", "free"), filepath.Join("..", "..", "..", "tests", "conformance", "expected")} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == "vendor" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(p, ".go") {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("no plugin tree next to the codemod")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if statusBackground.Match(b) {
			t.Errorf("%s: migrate.Status on an unbounded context", f)
		}
	}
}
