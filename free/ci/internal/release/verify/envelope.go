package verify

// Purpose: the DSSE envelope of the manifest and of the provenance: strict
// parse, signature check with the trust roots, and the mapping of signing
// errors to reason keys (Epic CACHE D16 `signature`, `key_purpose`, `revoked`).
// Inputs:  envelope bytes, the Verifier built from the trust roots.
// Outputs: the payload and the key id that signed it, or a Failure.
// Constraints: exactly one signature (v1); the key must be a trusted ci-release
// key; a key id of another purpose is key_purpose, never a silent unknown key;
// the signed payload is the only thing decoded, and only after the signature
// verified.

import (
	"encoding/json"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/provenance"
	"github.com/nself-org/plugins/free/ci/internal/release/manifest"
)

// parseEnvelope decodes a DSSE envelope document.
func parseEnvelope(b []byte) (signing.Envelope, error) {
	var env signing.Envelope
	if err := provenance.CheckStrict(b); err != nil {
		return env, err
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return signing.Envelope{}, err
	}
	return env, nil
}

// otherPurposes are the key id prefixes of every purpose but ci-release.
var otherPurposes = []signing.Purpose{signing.PurposePlugins, signing.PurposeAgent, signing.PurposeCINode, signing.PurposeCIAudit}

// foreignPurpose returns the purpose of a key id that is not a ci-release key.
func foreignPurpose(id string) (signing.Purpose, bool) {
	for _, p := range otherPurposes {
		if strings.HasPrefix(id, string(p)+"-") {
			return p, true
		}
	}
	return "", false
}

// verifyEnvelope checks env under v and returns the payload and signer. The
// reason is the key reported on any failure.
func verifyEnvelope(v *signing.Verifier, env signing.Envelope, reason string) ([]byte, string, *Failure) {
	if len(env.Signatures) != 1 {
		return nil, "", fail(reason, "the envelope holds %d signatures, v1 has exactly one", len(env.Signatures))
	}
	_, payload, ids, err := signing.VerifyEnvelope(v, env)
	if err != nil {
		return nil, "", mapSigningError(err, env, reason)
	}
	if len(ids) != 1 {
		return nil, "", fail(reason, "the envelope has %d verified signatures, v1 has exactly one", len(ids))
	}
	return payload, ids[0], nil
}

// mapSigningError turns a signing failure into a reason; only the manifest
// signature distinguishes a revoked key and a key of another purpose.
func mapSigningError(err error, env signing.Envelope, reason string) *Failure {
	if reason != ReasonSignature {
		return fail(reason, "%v", err)
	}
	if errIs(err, signing.ErrRevoked) {
		return fail(ReasonRevoked, "%v", err)
	}
	if errIs(err, signing.ErrWrongPurpose) {
		return fail(ReasonKeyPurpose, "%v", err)
	}
	if errIs(err, signing.ErrUnknownKey) {
		if p, ok := foreignPurpose(env.Signatures[0].KeyID); ok {
			return fail(ReasonKeyPurpose, "the manifest is signed by a %s key, not a ci-release key", p)
		}
	}
	return fail(ReasonSignature, "%v", err)
}

// signature reads the manifest envelope and verifies it under the trust roots.
func (r *run) signature() *Failure {
	b, err := readFile(r.dir, ManifestFile, maxEnvelope)
	if err != nil {
		return fail(ReasonEnvelope, "cannot read %s: %v", ManifestFile, err)
	}
	env, err := parseEnvelope(b)
	if err != nil {
		return fail(ReasonEnvelope, "%s is not a DSSE envelope: %v", ManifestFile, err)
	}
	if env.PayloadType != manifest.PayloadType {
		return fail(ReasonPayloadType, "payloadType is %q, want %q", clip(env.PayloadType), manifest.PayloadType)
	}
	if r.verifier, err = r.trustVerifier(); err != nil {
		return fail(ReasonSignature, "%v", err)
	}
	payload, keyID, f := verifyEnvelope(r.verifier, env, ReasonSignature)
	if f != nil {
		return f
	}
	r.payload, r.res.KeyID = payload, keyID
	return nil
}

// decode decodes the verified payload and binds it to the signer.
func (r *run) decode() *Failure {
	m, err := manifest.Decode(r.payload)
	if errIs(err, manifest.ErrSchema) {
		return fail(ReasonManifestSchema, "%v", err)
	}
	if err != nil {
		return fail(ReasonManifestInvalid, "%v", err)
	}
	if m.Signing.KeyID != r.res.KeyID {
		return fail(ReasonSignature, "the manifest names signing key %q, the envelope is signed by %q", m.Signing.KeyID, r.res.KeyID)
	}
	r.man = m
	r.res.Revision, r.res.RepoID, r.res.RunID, r.res.Artifacts = m.Revision, m.RepoID, m.Release.RunID, len(m.Artifacts)
	return nil
}

// clip shortens attacker-controlled text before it reaches a message.
func clip(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
