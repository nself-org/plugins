package verify

// Purpose: the supply-chain half of agent artifact verification (hq CACHE D17, P7-CACHE-10). Next to every binary a release
//          carries two SBOMs and a signed in-toto provenance statement; VerifyProvenance proves they belong to the manifest's
//          binary before verify.Artifact returns nil.
// Inputs:  the verified Manifest and Files (the bytes of <binary>.cdx.json, <binary>.spdx.json and <binary>.intoto.dsse.json).
// Outputs: nil, or an *Error with code E665 and reason "provenance" whose message says which rule failed.
// Constraints: the DSSE envelope is checked by cli sdk/go/signing under the agent purpose (a ci-release, plugins or unknown key is
//          refused); the statement is parsed by internal/provenance; the single subject is the binary named from the signed
//          target with the signed manifest's sha256; the byproducts are exactly the two SBOMs and carry the sha256 of the SBOM
//          bytes given here, and those bytes are SBOM documents that name the binary (sbom.go); the envelope is signed by the key of
//          the manifest unless the verifier is a rehearsal one (AllowRehearsalKeys). Nothing is read from disk, so the caller decides where the three files come from.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"
	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

// Size bounds of the three files.
const (
	MaxProvenanceBytes = 1 << 20
	MaxSBOMBytes       = 16 << 20
)

// File name suffixes next to a binary: nself-ci-agent-<target><suffix>.
const (
	SuffixCycloneDX  = ".cdx.json"
	SuffixSPDX       = ".spdx.json"
	SuffixProvenance = ".intoto.dsse.json"
)

// Files are the release files that sit next to one agent binary.
type Files struct {
	Provenance []byte // DSSE envelope (JSON) over the in-toto statement
	CycloneDX  []byte // CycloneDX 1.6 SBOM
	SPDX       []byte // SPDX SBOM
}

// BinaryName is the release name of the agent binary for target ("nself-ci-agent-linux-amd64").
func BinaryName(target string) string { return "nself-ci-agent-" + target }

func provFail(format string, a ...any) *Error { return fail(ReasonProvenance, nil, format, a...) }

// VerifyProvenance checks files against the verified manifest m, which the caller must already have verified (see the file comment). It does not look at the binary: its
// digest was proven equal to m.SHA256 by checkManifest.
func (v *Verifier) VerifyProvenance(m Manifest, files Files) error {
	if err := v.Ready(); err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		b    []byte
		max  int
	}{{"provenance", files.Provenance, MaxProvenanceBytes}, {"CycloneDX SBOM", files.CycloneDX, MaxSBOMBytes}, {"SPDX SBOM", files.SPDX, MaxSBOMBytes}} {
		if len(f.b) == 0 {
			return provFail("the release has no %s file", f.name)
		}
		if len(f.b) > f.max {
			return provFail("the %s file is larger than %d bytes", f.name, f.max)
		}
	}
	payload, signers, err := v.openEnvelope(files.Provenance)
	if err != nil {
		return err
	}
	if !v.rehearsal && !slices.Contains(signers, m.KeyID) {
		return provFail("the provenance is not signed by the key of the manifest (%s)", clip(m.KeyID))
	}
	st, err := provenance.Parse(payload)
	if err != nil {
		return fail(ReasonProvenance, err, "the provenance statement is not valid")
	}
	return checkStatement(st, m, files, v.tagsOnly)
}

// openEnvelope parses the DSSE envelope strictly and verifies its signature under the agent purpose. It returns the payload and the
// ids of the keys that signed it.
func (v *Verifier) openEnvelope(raw []byte) ([]byte, []string, error) {
	if err := provenance.CheckStrict(raw); err != nil {
		return nil, nil, fail(ReasonProvenance, err, "the provenance envelope is not one JSON object with distinct fields")
	}
	var env signing.Envelope
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		return nil, nil, provFail("the provenance file is not a DSSE envelope")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, nil, provFail("the provenance envelope has data after the object")
	}
	ptype, payload, signers, err := signing.VerifyEnvelope(v.sv, env)
	if err != nil {
		return nil, nil, fail(ReasonProvenance, err, "the provenance signature is refused (%s)", signingFailure(err, "").Reason)
	}
	if ptype != provenance.PayloadType {
		return nil, nil, provFail("the provenance payload type %q is not %s", clip(ptype), provenance.PayloadType)
	}
	return payload, signers, nil
}

// checkStatement binds a verified statement to the manifest and to the SBOM bytes.
func checkStatement(st provenance.Statement, m Manifest, files Files, tagsOnly bool) error {
	if st.Predicate.BuildDefinition.BuildType != provenance.BuildTypeAgent {
		return provFail("the provenance build type is %q, not the agent build type", clip(st.Predicate.BuildDefinition.BuildType))
	}
	if err := checkBuilder(st, m, tagsOnly); err != nil {
		return err
	}
	name := BinaryName(m.Target)
	if len(st.Subject) != 1 || st.Subject[0].Name != name || st.Subject[0].Digest["sha256"] != m.SHA256 {
		return provFail("the provenance subject is not %s with the signed sha256", name)
	}
	byproducts := st.Predicate.RunDetails.Byproducts
	if len(byproducts) != 2 {
		return provFail("the provenance lists %d byproducts, not the two SBOMs", len(byproducts))
	}
	for _, sbom := range []struct {
		suffix string
		b      []byte
	}{{SuffixCycloneDX, files.CycloneDX}, {SuffixSPDX, files.SPDX}} {
		want, ok := st.Byproduct(name + sbom.suffix)
		sum := sha256.Sum256(sbom.b)
		if !ok || want != hex.EncodeToString(sum[:]) {
			return provFail("the %s file is not the SBOM the provenance names", name+sbom.suffix)
		}
	}
	if err := checkCycloneDX(files.CycloneDX, name); err != nil {
		return err
	}
	return checkSPDX(files.SPDX, name)
}

// ReleaseRepo is the repository whose release workflow builds the agent.
const ReleaseRepo = "nself-org/plugins"

// checkBuilder pins the builder: the release workflow of ReleaseRepo, at the ref the statement names. A tag ref must be the manifest's
// version; the production verifier (tagsOnly, Default) accepts no other ref, a New verifier also accepts a branch ref (a rehearsal
// runs from a branch).
func checkBuilder(st provenance.Statement, m Manifest, tagsOnly bool) error {
	bd := st.Predicate.BuildDefinition
	repo := "https://github.com/" + ReleaseRepo
	ref := bd.ExternalParameters["ref"]
	if bd.ExternalParameters["repository"] != repo || bd.ExternalParameters["workflow"] != provenance.AgentWorkflow ||
		st.Predicate.RunDetails.Builder.ID != repo+"/"+provenance.AgentWorkflow+"@"+ref {
		return provFail("the provenance builder is not the agent release workflow of %s", ReleaseRepo)
	}
	if (tagsOnly || strings.HasPrefix(ref, "refs/tags/")) && ref != "refs/tags/v"+m.Version {
		return provFail("the provenance was built from %s, not the tag of version %s", clip(ref), clip(m.Version))
	}
	return nil
}
