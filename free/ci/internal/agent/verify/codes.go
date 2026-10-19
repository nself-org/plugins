package verify

// Purpose: the code and the typed error of agent artifact verification (hq NODE D15, P7-NODE-15).
// Inputs:  none.
// Outputs: E665 registered through model.Register; *Error carrying the code and a stable reason key.
// Constraints: one code, E665, for every failure; the reason key tells the causes apart. Messages hold
//          reason text, key ids and versions only, never key bytes or file contents.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeVerify is the code of every agent artifact verification failure.
const CodeVerify = "E665"

// Reason keys carried by Error.Reason. Each is a distinct way an artifact fails.
const (
	ReasonNoTrustedKey      = "no_trusted_key"      // no agent public key is built in (nothing to verify against)
	ReasonMissingSignature  = "missing_signature"   // the manifest carries no key id or no signature
	ReasonMalformedManifest = "malformed_manifest"  // not an agent-release/v1 manifest in canonical form
	ReasonMalformedSig      = "malformed_signature" // the signature text is not canonical base64 of 64 bytes
	ReasonUnknownKey        = "unknown_key"         // the key id is not in the trusted set
	ReasonWrongPurpose      = "wrong_purpose"       // the key is not an agent-purpose key (a plugin key, for example)
	ReasonRevoked           = "revoked"             // the key id is on the revoked list
	ReasonKeyNotValid       = "key_not_valid"       // the key is expired or not yet valid
	ReasonBadSignature      = "bad_signature"       // the signature does not verify over the signing input
	ReasonVersionMismatch   = "version_mismatch"    // the signed version is not the pinned version
	ReasonTargetMismatch    = "target_mismatch"     // the signed target is not the node's target
	ReasonChecksumMismatch  = "checksum_mismatch"   // the file's sha256 is not the signed sha256 (tampered or truncated)
	ReasonUnreadable        = "unreadable_artifact" // the file cannot be read as a bounded regular file
)

// Error is a verification failure.
type Error struct {
	Reason string
	Msg    string
	Err    error // the underlying sdk/go/signing sentinel, when there is one
}

// Code returns E665.
func (e *Error) Code() string { return CodeVerify }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s: %s", CodeVerify, e.Reason, e.Msg) }

// Unwrap exposes the signing sentinel so callers can use errors.Is.
func (e *Error) Unwrap() error { return e.Err }

func fail(reason string, err error, format string, a ...any) *Error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...), Err: err}
}

// ReasonOf returns the reason key of a verification error, or "".
func ReasonOf(err error) string {
	var ve *Error
	if errors.As(err, &ve) {
		return ve.Reason
	}
	return ""
}

func init() {
	model.Register(model.Code{ID: CodeVerify, Class: "auth", Summary: "agent artifact verification failed (reason keys no_trusted_key, missing_signature, unknown_key, wrong_purpose, revoked, bad_signature, version_mismatch, target_mismatch, checksum_mismatch)",
		Fix: "Install the agent from a signed release of the pinned version, or pass --agent-binary <path> with a binary you trust"})
}
