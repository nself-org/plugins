package sbom_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

var twoTargets = []sbom.Target{
	{Name: "cli", Path: "dist/cli", Kind: sbom.SourceFile},
	{Name: "web", Path: "out", Kind: sbom.SourceDir},
}

func TestGenerateReturnsBothFormatsWithDigests(t *testing.T) {
	r := &fakeRunner{onScan: scanReturning(t, nil)}
	docs, err := sbom.Generate(context.Background(), r, "/s/syft", testPin(), twoTargets)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || len(r.cmds) != 2 {
		t.Fatalf("docs=%d cmds=%d", len(docs), len(r.cmds))
	}
	for i, d := range docs {
		wantCDX, wantSPDX := sbom.Outputs(twoTargets[i].Name)
		if d.Name != twoTargets[i].Name || d.CDXPath != wantCDX || d.SPDXPath != wantSPDX {
			t.Errorf("doc %d = %+v", i, d)
		}
		if len(d.CDXSHA256) != 64 || len(d.SPDXSHA256) != 64 || d.CDXSHA256 == d.SPDXSHA256 || len(d.CDX) == 0 || len(d.SPDX) == 0 {
			t.Errorf("doc %d digests/bytes: %+v", i, d)
		}
	}
	if docs[0].CDXSHA256 == docs[1].CDXSHA256 {
		t.Error("different artifacts share a digest")
	}
}

func TestGenerateFailuresAreE718Invalid(t *testing.T) {
	ctx := context.Background()
	one := twoTargets[:1]
	cases := []struct {
		name string
		run  func(sbom.Cmd) (sbom.Result, error)
		want string
	}{
		{"syft exits non-zero", func(sbom.Cmd) (sbom.Result, error) { return sbom.Result{Exit: 1, Output: "boom: bad source"}, nil }, "boom"},
		{"syft cannot run", func(sbom.Cmd) (sbom.Result, error) { return sbom.Result{}, errors.New("sandbox denied") }, "sandbox denied"},
		{"no files written", func(sbom.Cmd) (sbom.Result, error) { return sbom.Result{}, nil }, "wrote no"},
		{"cdx for another artifact", scanReturning(t, func(d string) string { return strings.Replace(d, `"name": "cli"`, `"name": "elsewhere"`, 1) }), "metadata.component.name"},
		{"cdx from another syft", scanReturning(t, func(d string) string { return strings.Replace(d, `"version": "1.54.1"`, `"version": "9.9.9"`, 1) }), "metadata.tools"},
		{"cdx not CycloneDX 1.6", scanReturning(t, func(d string) string { return strings.Replace(d, `"1.6"`, `"1.4"`, 1) }), "specVersion"},
	}
	for _, c := range cases {
		_, err := sbom.Generate(ctx, &fakeRunner{onScan: c.run}, "syft", testPin(), one)
		if !sbom.IsReason(err, sbom.ReasonInvalid) || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "cli") {
			t.Errorf("%s: got %v, want E718 invalid naming the artifact and %q", c.name, err, c.want)
		}
	}
}

func TestGenerateRefusesBadInputBeforeRunning(t *testing.T) {
	r := &fakeRunner{onScan: scanReturning(t, nil)}
	for name, targets := range map[string][]sbom.Target{
		"none":      nil,
		"bad path":  {{Name: "a", Path: "../x", Kind: sbom.SourceFile}},
		"duplicate": {{Name: "a", Path: "x", Kind: sbom.SourceFile}, {Name: "a", Path: "y", Kind: sbom.SourceFile}},
	} {
		if _, err := sbom.Generate(context.Background(), r, "syft", testPin(), targets); !sbom.IsReason(err, sbom.ReasonInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if len(r.cmds) != 0 {
		t.Fatalf("syft ran for refused input: %v", r.cmds)
	}
}

func TestGenerateRefusesAnEmptyInventoryUnlessAllowed(t *testing.T) {
	strip := scanReturning(t, func(d string) string {
		var m map[string]any
		if err := json.Unmarshal([]byte(d), &m); err != nil {
			t.Fatal(err)
		}
		delete(m, "components")
		b, _ := json.Marshal(m)
		return string(b)
	})
	one := []sbom.Target{{Name: "cli", Path: "dist/cli", Kind: sbom.SourceFile}}
	_, err := sbom.Generate(context.Background(), &fakeRunner{onScan: strip}, "syft", testPin(), one)
	if !sbom.IsReason(err, sbom.ReasonInvalid) || !strings.Contains(err.Error(), "no components") {
		t.Fatalf("empty SBOM: %v", err)
	}
	one[0].AllowEmpty = true
	if _, err := sbom.Generate(context.Background(), &fakeRunner{onScan: strip}, "syft", testPin(), one); err != nil {
		t.Fatalf("AllowEmpty: %v", err)
	}
}

func TestGenerateRefusesNamesThatCollideOnACaseFoldingFileSystem(t *testing.T) {
	r := &fakeRunner{onScan: scanReturning(t, nil)}
	_, err := sbom.Generate(context.Background(), r, "syft", testPin(), []sbom.Target{
		{Name: "App", Path: "a", Kind: sbom.SourceFile}, {Name: "app", Path: "b", Kind: sbom.SourceFile}})
	if !sbom.IsReason(err, sbom.ReasonInvalid) || len(r.cmds) != 0 {
		t.Fatalf("got %v, cmds %d", err, len(r.cmds))
	}
}

func TestGenerateRefusesSPDXOfAnotherArtifact(t *testing.T) {
	good := scanReturning(t, nil)
	run := func(c sbom.Cmd) (sbom.Result, error) {
		res, _ := good(c)
		res.Files[c.Collect[1]] = []byte(strings.Replace(string(res.Files[c.Collect[1]]), `"name": "cli"`, `"name": "elsewhere"`, 1))
		return res, nil
	}
	_, err := sbom.Generate(context.Background(), &fakeRunner{onScan: run}, "syft", testPin(), []sbom.Target{{Name: "cli", Path: "x", Kind: sbom.SourceFile}})
	if !sbom.IsReason(err, sbom.ReasonInvalid) || !strings.Contains(err.Error(), "SPDX name") {
		t.Fatalf("got %v", err)
	}
}
