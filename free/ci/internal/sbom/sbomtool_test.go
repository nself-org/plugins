//go:build sbomtool

package sbom_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

// componentNames lists the names in a CycloneDX document.
func componentNames(t *testing.T, doc []byte) map[string]bool {
	t.Helper()
	var d struct {
		Components []struct{ Name string } `json:"components"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, c := range d.Components {
		out[c.Name] = true
	}
	return out
}

// TestSyftFixtures needs the pinned syft on PATH (a prerequisite, so it fails
// rather than skips): a Go binary, a pnpm package tarball and an OCI image
// archive each yield CycloneDX 1.6 that matches the vendored schema and SPDX
// 2.3 that parses, and the Go and pnpm documents list their direct dependency.
func TestSyftFixtures(t *testing.T) {
	ctx := context.Background()
	pin, err := sbom.EmbeddedPin()
	if err != nil {
		t.Fatal(err)
	}
	fx := t.TempDir()
	buildGoFixture(t, fx)
	writePnpmTarball(t, fx+"/fixture-pkg.tgz")
	writeOCIArchive(t, fx+"/fixture-img.tar")
	r := execRunner(t, fx)
	syft, err := sbom.Resolve(ctx, r, t.TempDir(), sbom.ModeContainer, pin)
	if err != nil {
		t.Fatalf("syft %s must be on PATH for the sbomtool tests: %v", pin.Version, err)
	}
	targets := []sbom.Target{
		{Name: "gobin", Path: "gobin", Kind: sbom.SourceFile},
		{Name: "fixture-pkg", Path: "fixture-pkg.tgz", Kind: sbom.SourceFile},
		{Name: "fixture-img", Path: "fixture-img.tar", Kind: sbom.SourceImage},
	}
	docs, err := sbom.Generate(ctx, r, syft, pin, targets)
	if err != nil {
		t.Fatal(err)
	}
	rs := cdxSchema(t)
	want := map[string]string{"gobin": "gopkg.in/yaml.v3", "fixture-pkg": "left-pad", "fixture-img": "busybox"}
	for _, d := range docs {
		assertCycloneDXSchema(t, rs, d.CDX)
		if err := sbom.ValidateSPDX(d.SPDX); err != nil {
			t.Errorf("%s: %v", d.Name, err)
		}
		if names := componentNames(t, d.CDX); !names[want[d.Name]] {
			t.Errorf("%s: components %v do not list %s", d.Name, names, want[d.Name])
		}
	}
}

// TestSyftVersionMismatchIsToolVersion proves a pin that differs from the real
// syft is refused before anything is generated.
func TestSyftVersionMismatchIsToolVersion(t *testing.T) {
	r := execRunner(t, t.TempDir())
	_, err := sbom.Resolve(context.Background(), r, t.TempDir(), sbom.ModeContainer, sbom.Pin{Version: "0.0.1"})
	if !sbom.IsReason(err, sbom.ReasonToolVersion) || !strings.Contains(err.Error(), sbom.InstallCommand) {
		t.Fatalf("got %v, want E718 tool_version naming the install command", err)
	}
}
