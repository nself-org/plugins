package attest

// Purpose: the error code and the stable reason keys of a runner signature that did not verify (Epic D25:
// E686 "signature invalid, unknown key or replay").
// Inputs:  none (registration at init).
// Outputs: RemoteReason, the Remote* constants, RemoteError.
// Constraints: reason keys are API: a consumer branches on them, so a key is never renamed or reused. The
//          gate sees one reason (evidence.signature_unverifiable); the key tells the operator why.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeSignatureRejected is E686.
const CodeSignatureRejected = "E686"

func init() {
	model.Register(model.Code{ID: CodeSignatureRejected, Class: "usage", Summary: "evidence signature invalid, key unknown or signature replayed",
		Fix: "Run the job again on a registered node whose evidence key is not revoked, or fix the reason named in the error"})
}

// RemoteReason is one stable reason a runner signature was refused.
type RemoteReason string

// The reasons, in the order the verifier checks them.
const (
	RemoteSignatureMissing    RemoteReason = "signature_missing"
	RemoteAttemptUnknown      RemoteReason = "attempt_unknown"
	RemoteRevisionMismatch    RemoteReason = "revision_mismatch"
	RemoteReplay              RemoteReason = "replay"
	RemoteKeyRevoked          RemoteReason = "key_revoked"
	RemoteKeyUnknown          RemoteReason = "key_unknown"
	RemoteKeyPurpose          RemoteReason = "key_purpose"
	RemoteKeyMalformed        RemoteReason = "key_malformed"
	RemoteSignatureMalformed  RemoteReason = "signature_malformed"
	RemoteSignatureInvalid    RemoteReason = "signature_invalid"
	RemoteLeaseHolderMismatch RemoteReason = "lease_holder_mismatch"
	RemoteNodeNotEligible     RemoteReason = "node_not_eligible"
	RemoteLedgerUnavailable   RemoteReason = "ledger_unavailable"
)

// RemoteError is the error form of a refusal; it carries code E686.
type RemoteError struct {
	Reason RemoteReason
	Detail string
	Err    error
}

func (e *RemoteError) Error() string {
	msg := fmt.Sprintf("%s: runner signature refused: %s", CodeSignatureRejected, e.Reason)
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

func (e *RemoteError) Unwrap() error { return e.Err }

func remoteFail(r RemoteReason, detail string, err error) error {
	return &RemoteError{Reason: r, Detail: detail, Err: err}
}

// RemoteReasonOf returns the reason of a refusal, or "" when err is not one.
func RemoteReasonOf(err error) RemoteReason {
	var re *RemoteError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ""
}
