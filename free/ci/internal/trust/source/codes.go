package source

// Purpose: E688 "approval missing or void" and its reason keys (Epic D18, D25).
// Inputs:  none.
// Outputs: *ApprovalError with a stable reason key; the code registered through model.Register.
// Constraints: registered once, here, never by a panic; a message holds reason and ids only, never a token.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeApproval is E688.
const CodeApproval = "E688"

// Reason keys carried by ApprovalError.Reason.
const (
	ApprovalSelf          = "approval.self"           // the author tried to approve their own run
	ApprovalAuthorUnknown = "approval.author_unknown" // no author fact: nobody else can be told from the author
	ApprovalUnknownRun    = "approval.unknown_run"
	ApprovalNotRequired   = "approval.not_required"
	ApprovalMissing       = "approval.missing"
	ApprovalVoid          = "approval.void"
	ApprovalBadBinding    = "approval.bad_binding"
)

func init() {
	model.Register(model.Code{ID: CodeApproval, Class: "usage", Summary: "run approval missing, void or refused",
		Fix: "Ask a release or admin user other than the author to run `nself ci runs approve <run>` for the current head"})
}

// ApprovalError is an E688 failure.
type ApprovalError struct{ Reason, Msg string }

func (e *ApprovalError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", CodeApproval, e.Msg, e.Reason)
}

// ErrCode and ErrReason let a handler map the error to its JSON body.
func (e *ApprovalError) ErrCode() string   { return CodeApproval }
func (e *ApprovalError) ErrReason() string { return e.Reason }

// NewApprovalError builds an E688 with a reason key.
func NewApprovalError(reason, format string, a ...any) error {
	return &ApprovalError{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ApprovalReasonOf returns the E688 reason key of err, or "" when err is not an E688 error.
func ApprovalReasonOf(err error) string {
	var e *ApprovalError
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
