package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestReferenceListsEveryManifestCommandAndFlag fails when a command of
// plugin.json, a registered sub-verb or a defined flag is missing from the
// generated Reference.md.
func TestReferenceListsEveryManifestCommandAndFlag(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	out, err := generate(root)
	if err != nil {
		t.Fatal(err)
	}
	ref := string(out[referencePage])
	mf, err := readManifest(filepath.Join(root, ciDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	idx := map[string]manifestCommand{}
	manifestIndex(mf.Commands.Subcommands, "", idx)
	for key := range idx {
		if !strings.Contains(ref, "| `nself ci "+key+"` |") {
			t.Errorf("command %q is in plugin.json but not in Reference.md", key)
		}
	}
	for _, key := range []string{"runs approve", "runs pending"} {
		if !strings.Contains(ref, "| `nself ci "+key+"` |") {
			t.Errorf("sub-verb %q missing from Reference.md", key)
		}
	}
	trust := string(out[trustPage])
	if !strings.Contains(trust, begin("trust-reasons")) || !strings.Contains(trust, end("trust-reasons")) {
		t.Error("trust-reasons markers missing from the trust guide")
	}
	again, err := generate(root)
	if err != nil {
		t.Fatal(err)
	}
	for rel, b := range out {
		if string(again[rel]) != string(b) {
			t.Errorf("%s: generation is not deterministic", rel)
		}
	}
	if !strings.Contains(ref, "| `--allow-private` |") {
		t.Error("flag --allow-private missing from Reference.md")
	}
}
