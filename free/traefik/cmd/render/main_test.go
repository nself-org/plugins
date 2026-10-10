package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fx(name string) string {
	return filepath.Join("..", "..", "testdata", "fixtures", name, "routes.json")
}

func TestRunExitCodesAndNoPartialFile(t *testing.T) {
	ssl := t.TempDir() // no lineages: only HTTP-only fixtures render
	out := t.TempDir()
	var errb bytes.Buffer
	if rc := run([]string{"--routes", fx("http-full"), "--out", out, "--ssl", ssl}, &errb); rc != 0 {
		t.Fatalf("http-full: exit %d: %s", rc, errb.String())
	}
	for _, f := range []string{"dynamic.yml", ".nself-generated"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Fatalf("missing %s", f)
		}
	}
	ents, _ := os.ReadDir(out)
	if len(ents) != 2 {
		t.Fatalf("temp files left behind: %v", ents)
	}
	// a refusal exits 1, names the route and field, and writes nothing
	out2 := t.TempDir()
	errb.Reset()
	rc := run([]string{"--routes", fx("plugin-unmodelled"), "--out", out2, "--ssl", ssl}, &errb)
	if rc != 1 || !strings.Contains(errb.String(), "plugin:idme/idme.conf#2") || !strings.Contains(errb.String(), "unmodelled") {
		t.Fatalf("refusal: exit %d: %s", rc, errb.String())
	}
	if ents, _ := os.ReadDir(out2); len(ents) != 0 {
		t.Fatalf("a refusal wrote files: %v", ents)
	}
	// unreadable input is an I/O error, not a refusal
	if rc := run([]string{"--routes", filepath.Join(out2, "nope.json"), "--out", out2}, &errb); rc != 2 {
		t.Fatalf("missing input: exit %d", rc)
	}
	// a file that is not routes.json is invalid input
	bad := filepath.Join(out2, "bad.json")
	_ = os.WriteFile(bad, []byte(`{"schema_version":"1"}`), 0o644)
	if rc := run([]string{"--routes", bad, "--out", out2}, &errb); rc != 1 {
		t.Fatalf("invalid input: exit %d", rc)
	}
}
