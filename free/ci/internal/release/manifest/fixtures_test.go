package manifest_test

// Purpose: shared fixtures for the manifest tests: one valid manifest with a
// binary and an image, and helpers to bend it.

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

func hex64(c string) string { return strings.Repeat(c, 64) }

func strp(s string) *string { return &s }

func file(path, c string) manifest.FileRef { return manifest.FileRef{Path: path, SHA256: hex64(c)} }

func sbomFor(name, c string) manifest.SBOM {
	return manifest.SBOM{CycloneDX: file("sbom/"+name+".cdx.json", c), SPDX: file("sbom/"+name+".spdx.json", c)}
}

// valid returns a manifest every rule accepts; its artifacts are deliberately
// out of order so Build has to sort them.
func valid() manifest.Manifest {
	return manifest.Manifest{
		Schema: manifest.Schema, RepoID: "repo-1", Revision: strings.Repeat("a", 40),
		Release: manifest.Release{RunID: "run-1", Pipeline: "release", JobID: "publish", AttemptID: "run-1.publish-1a2b3c4d.a1",
			IdempotencyKey: hex64("9"), FinishedAt: "2026-10-19T12:00:00Z"},
		Evidence: manifest.EvidenceRef{Path: manifest.EvidencePath, SHA256: hex64("1"), Result: manifest.EvidenceResult, Class: manifest.EvidenceClass},
		Artifacts: []manifest.Artifact{
			{Name: "nself-linux-amd64", Path: "artifacts/nself-linux-amd64", Kind: manifest.KindBinary, SHA256: hex64("2"), Size: 1234,
				Platform: strp("linux/amd64"), SBOM: sbomFor("nself-linux-amd64", "3")},
			{Name: "image/nself", Path: "artifacts/image/nself", Kind: manifest.KindImage, SHA256: hex64("4"), Size: 99,
				Image: &manifest.Image{Ref: "ghcr.io/example/nself:1.0.0", IndexDigest: "sha256:" + hex64("5"), Manifests: []manifest.ImageManifest{
					{Platform: "linux/arm64", Digest: "sha256:" + hex64("7")}, {Platform: "linux/amd64", Digest: "sha256:" + hex64("6")}}},
				SBOM: sbomFor("image/nself", "8")},
		},
		Provenance: manifest.Provenance{Path: manifest.ProvenancePath, SHA256: hex64("a"), PredicateType: manifest.ProvenancePredicate, SLSABuildLevel: manifest.SLSALevel},
		Checksums:  file(manifest.ChecksumsPath, "b"),
		Signing:    manifest.Signing{Purpose: manifest.SigningPurpose, KeyID: "ci-release-0123456789abcdef"},
		Producer:   manifest.Producer{NselfCIVersion: "1.5.0", Coordinator: "coordinator-1"},
	}
}

func mustBuild(t testing.TB, m manifest.Manifest) []byte {
	t.Helper()
	b, err := manifest.Build(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
