package verify

// Purpose: regression tests for D-1243: the provenance must be signed by the key of the manifest (outside a rehearsal), and the two
//          SBOMs must be SBOM documents of the right format that name the binary, not just bytes whose digest the provenance holds.
// Constraints: every key is a throwaway generated here; no network; no secret.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

// cdxDoc and spdxDoc are the smallest documents of each format that name the binary.
func cdxDoc(name string) string {
	return fmt.Sprintf(`{"bomFormat":"CycloneDX","specVersion":"1.6","metadata":{"component":{"type":"file","name":%q}}}`, name)
}

func spdxDoc(name string) string {
	return fmt.Sprintf(`{"spdxVersion":"SPDX-2.3","name":%q}`, name)
}

// TestProvenanceKeyMustBeTheManifestKey: a second trusted agent key may not vouch for a manifest it did not sign.
func TestProvenanceKeyMustBeTheManifestKey(t *testing.T) {
	manifestKey := newSigner(t, signing.PurposeAgent)
	provKey := newSigner(t, signing.PurposeAgent)
	r := newRelease(t, manifestKey)
	r.files.Provenance = envelopeBytes(t, provKey, provenance.PayloadType, r.statement(t, nil))
	_, err := mustVerifier(t, nil, manifestKey.key(), provKey.key()).Artifact(r.path, r.json(t), pinned, r.files)
	wantReason(t, err, ReasonProvenance)
	if !strings.Contains(err.Error(), "key of the manifest") {
		t.Fatalf("expected the same-key rule, got %v", err)
	}
	// A rehearsal signs the manifest and the provenance with two throwaway keys.
	m, err := mustVerifier(t, nil, manifestKey.key(), provKey.key()).AllowRehearsalKeys().Artifact(r.path, r.json(t), pinned, r.files)
	if err != nil || m.SHA256 != r.m.SHA256 {
		t.Fatalf("a rehearsal verifier must accept two keys: %v", err)
	}
	// An envelope that also carries the manifest key's signature is signed by it.
	r.files.Provenance = envelopeBytes(t, manifestKey, provenance.PayloadType, r.statement(t, nil))
	if _, err = mustVerifier(t, nil, manifestKey.key(), provKey.key()).Artifact(r.path, r.json(t), pinned, r.files); err != nil {
		t.Fatalf("the manifest key's own provenance must verify: %v", err)
	}
}

// TestSBOMsMustBeSBOMsNamingTheBinary: a signed release may not carry bytes that are no SBOM, or the SBOM of another artifact.
func TestSBOMsMustBeSBOMsNamingTheBinary(t *testing.T) {
	agent := newSigner(t, signing.PurposeAgent)
	good := newRelease(t, agent)
	name := BinaryName(good.m.Target)
	// signed rebuilds the release with the given SBOM bytes, bound by digest in a provenance the agent key signed.
	signed := func(cdx, spdx string) release {
		r := good
		r.files.CycloneDX, r.files.SPDX = []byte(cdx), []byte(spdx)
		r.files.Provenance = envelopeBytes(t, agent, provenance.PayloadType, r.statement(t, nil))
		return r
	}
	if _, err := mustVerifier(t, nil, agent.key()).Artifact(good.path, good.json(t), pinned, signed(cdxDoc(name), spdxDoc(name)).files); err != nil {
		t.Fatalf("well-formed SBOMs naming the binary must verify: %v", err)
	}
	cases := []struct {
		name, cdx, spdx, why string
	}{
		{"CycloneDX is not JSON", "not json", spdxDoc(name), "CycloneDX"},
		{"CycloneDX has trailing data", cdxDoc(name) + "{}", spdxDoc(name), "CycloneDX"},
		{"CycloneDX is another format", `{"spdxVersion":"SPDX-2.3","name":"` + name + `"}`, spdxDoc(name), "CycloneDX"},
		{"CycloneDX is another spec version", strings.Replace(cdxDoc(name), `"1.6"`, `"1.4"`, 1), spdxDoc(name), "CycloneDX"},
		{"CycloneDX names another binary", cdxDoc("nself-ci-agent-darwin-arm64"), spdxDoc(name), "CycloneDX"},
		{"CycloneDX names no component", `{"bomFormat":"CycloneDX","specVersion":"1.6"}`, spdxDoc(name), "CycloneDX"},
		{"SPDX is not JSON", cdxDoc(name), "not json", "SPDX"},
		{"SPDX is another format", cdxDoc(name), cdxDoc(name), "SPDX"},
		{"SPDX is another version", cdxDoc(name), strings.Replace(spdxDoc(name), "SPDX-2.3", "SPDX-1.0", 1), "SPDX"},
		{"SPDX names another binary", cdxDoc(name), spdxDoc("nself-ci-agent-darwin-arm64"), "SPDX"},
		{"SPDX names nothing", cdxDoc(name), `{"spdxVersion":"SPDX-2.3"}`, "SPDX"},
	}
	for _, c := range cases {
		r := signed(c.cdx, c.spdx)
		_, err := mustVerifier(t, nil, agent.key()).Artifact(r.path, r.json(t), pinned, r.files)
		if err == nil {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		if ReasonOf(err) != ReasonProvenance || !strings.Contains(err.Error(), c.why+" SBOM") {
			t.Errorf("%s: want E665 provenance naming the %s SBOM, got %v", c.name, c.why, err)
		}
	}
}
