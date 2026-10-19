package pairing

// Purpose: E659 "pairing code invalid" and its reason keys (Epic D15), the error type of this package.
// Inputs:  none.
// Outputs: *Error with a stable reason key; the code registered through model.Register. E660 (certificate
//          invalid: clock skew, cert_ttl, fingerprint, node key) is owned by internal/pki and returned
//          as *pki.Error.
// Constraints: a message holds reasons and ids only, never a code, key or token.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeInvalid is E659.
const CodeInvalid = "E659"

// Reason keys carried by Error.Reason.
const (
	ReasonInvalid      = "invalid" // no such code
	ReasonExpired      = "expired" // past its 10 minutes
	ReasonLocked       = "locked"  // five wrong codes were presented
	ReasonReused       = "reused"  // redeemed before; the first certificate was revoked
	ReasonWithdrawn    = "withdrawn"
	ReasonDenied       = "denied" // the approver said no
	ReasonNoIntent     = "no_intent"
	ReasonExistingNode = "existing_node"
	ReasonNoApprover   = "no_approver"
	ReasonBadRequest   = "bad_request"
	ReasonInternal     = "internal"
)

func init() {
	model.Register(model.Code{ID: CodeInvalid, Class: "usage", Summary: "pairing code invalid", Fix: "Start a new pairing window with `nself ci nodes pair` and use the code it prints"})
}

// Error is an E659 failure.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string     { return fmt.Sprintf("%s: %s (%s)", CodeInvalid, e.Msg, e.Reason) }
func (e *Error) ErrCode() string   { return CodeInvalid }
func (e *Error) ErrReason() string { return e.Reason }

func fail(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the E659 reason key of err, or "" when err is not an E659 error.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
