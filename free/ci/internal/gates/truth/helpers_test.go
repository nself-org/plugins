package truth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTree writes files (slash paths) under a fresh temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// plantedHost is a documentation address (RFC 5737): a stand-in for a dead host
// that no real rule names. Tests build every dead-host literal at run time.
func plantedHost() string { return strings.Join([]string{"203", "0", "113", "7"}, ".") }

// plantedName is a host name under the reserved .invalid TLD.
func plantedName() string { return "gone" + ".example" + ".invalid" }

// fixtureRules is a rule file with one rule per kind, the dead host by hash only.
func fixtureRules() string {
	return `version: 1
rules:
  - id: no-secret-word
    kind: banned-literal
    literal: SEKRET_MARKER
    message: remove the marker
  - id: retired-oldname
    kind: retired-term
    term: Old Name
    message: say New Name
  - id: dead-ip
    kind: dead-host
    sha256: [` + HashLiteral(plantedHost()) + `]
    message: that address is gone
  - id: dead-name
    kind: dead-host
    sha256: [` + HashLiteral(plantedName()) + `]
    message: that host is gone
`
}

func mustErr(t *testing.T, err error, wantFile, wantKey string) {
	t.Helper()
	e, ok := err.(*Error)
	if !ok {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if e.Code() != "E644" || !strings.HasPrefix(e.Error(), "E644 ") {
		t.Errorf("code: %v", e)
	}
	if e.File != wantFile || e.Key != wantKey {
		t.Errorf("error names %q / %q, want %q / %q (%v)", e.File, e.Key, wantFile, wantKey, e)
	}
}
