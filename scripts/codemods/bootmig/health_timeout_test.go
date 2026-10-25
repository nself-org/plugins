package main

import (
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

// The generated copies in the plugins carry the same fix.
func TestGeneratedHelpersBoundStatus(t *testing.T) {
	root := filepath.Join("..", "..", "..", "free")
	var files []string
	for _, pat := range []string{"*/internal/boot_migrations.go", "*/cmd/boot_migrations.go"} {
		m, err := filepath.Glob(filepath.Join(root, pat))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
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
