package verify

// Purpose: E719, the one error code of release verification (Epic CACHE D16,
// D20), and the stable reason keys that say which check failed.
// Inputs:  none. Outputs: Code, the Reason* keys, Failure.
// Constraints: registered through model.Register, never a panic. A reason key
// is stable API: scripts and P7-DEPL-19 match on it. Reasons ReasonEvidenceFailing
// and ReasonQuarantined exit 1, every other reason exits 11.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Code is the error code of a release that did not verify.
const Code = "E719"

// Reason keys, in the order Release checks them.
const (
	ReasonEnvelope          = "envelope"                   // a file is missing, unreadable, too large or not the expected JSON
	ReasonPayloadType       = "payload_type"               // the manifest envelope has another payloadType
	ReasonSignature         = "signature"                  // no trusted ci-release key signed the manifest exactly once
	ReasonKeyPurpose        = "key_purpose"                // the signing key belongs to another purpose
	ReasonRevoked           = "revoked"                    // the signing key is revoked
	ReasonManifestSchema    = "manifest_schema"            // another manifest schema major
	ReasonManifestInvalid   = "manifest_invalid"           // the manifest breaks a v1 rule
	ReasonChecksum          = "checksum"                   // SHA256SUMS or a file digest does not match
	ReasonEvidenceMismatch  = "evidence_mismatch"          // the evidence is not the one the manifest names
	ReasonRevision          = "revision"                   // the evidence is for another revision
	ReasonProvenanceSig     = "provenance_signature"       // the provenance envelope is not signed by the manifest's key
	ReasonProvenanceSubject = "provenance_subject"         // the provenance describes other artifacts or another build
	ReasonSBOMLink          = "sbom_link"                  // an SBOM is missing or not bound to its artifact
	ReasonRevisionAbsent    = "revision_absent"            // the released revision is not in the repository
	ReasonEvidenceInvalid   = "evidence_invalid"           // the evidence document is malformed or inconsistent
	ReasonEvidenceStale     = "evidence_stale_at_revision" // the inputs at the released revision differ from the evidence
	ReasonEvidenceUnbound   = "evidence_unbound"           // the evidence names no clean commit
	ReasonEvidenceFailing   = "evidence_failing"           // the evidence does not pass (exit 1)
	ReasonQuarantined       = "quarantined"                // the evidence was quarantined (exit 1)
	ReasonReleaseMissing    = "release_missing"            // there is no release directory to check
)

// Exit statuses of `nself ci artifacts verify` (P7-CI D11 classes).
const (
	ExitVerified = 0
	exitFailing  = 1
	exitInvalid  = 11
)

func init() {
	model.Register(model.Code{ID: Code, Class: "usage", Summary: "release verification failed",
		Fix: "Read the reason key in the output; fetch the release again, or run `nself ci artifacts verify <dir> --json` for every check"})
}

// Failure is a release that did not verify: the first failing check.
type Failure struct{ Reason, Message string }

func (f *Failure) Error() string { return Code + " " + f.Reason + ": " + f.Message }

// ExitCode is the `ci artifacts verify` exit status of the failure.
func (f *Failure) ExitCode() int {
	if f.Reason == ReasonEvidenceFailing || f.Reason == ReasonQuarantined {
		return exitFailing
	}
	return exitInvalid
}

func fail(reason, format string, a ...any) *Failure {
	return &Failure{Reason: reason, Message: fmt.Sprintf(format, a...)}
}
