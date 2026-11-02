package truth

// Purpose: the shipped org rules (.github/truth/org.yaml) let an oversize vendored binary archive pass
// (the cli repo vendors wasmtime-go .a files above the 16 MiB read bound) while an oversize file anywhere
// else, or any other file under vendor, still fails closed as unscanned.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShippedRulesAllowOversizeVendoredArchivesOnly(t *testing.T) {
	rules, err := os.ReadFile("../../../../../.github/truth/org.yaml")
	if err != nil {
		t.Fatal(err)
	}
	root := writeTree(t, map[string]string{"org.yaml": string(rules)})
	sparse := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(maxHostScanBytes + 1); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	sparse("vendor/github.com/bytecodealliance/wasmtime-go/v25/build/linux-x86_64/libwasmtime.a")
	sparse("vendor/example.com/lib/build/windows/wasmtime.lib")
	sparse("build/libwasmtime.a")
	sparse("vendor/example.com/lib/blob.bin")
	rs, err := Load(root, []string{"org.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Scan(root, rs, rs.Files)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range res.Findings {
		if f.RuleID != "truth.unscanned-file" {
			t.Errorf("unexpected finding %+v", f)
		}
		got[f.Path] = true
	}
	if len(got) != 2 || !got["build/libwasmtime.a"] || !got["vendor/example.com/lib/blob.bin"] {
		t.Fatalf("only the non-vendor archive and the vendored non-archive may be flagged, got %+v", res.Findings)
	}
}
