// Package lifecycle is the node lifecycle behind `nself ci nodes add|list|show|test|drain|enable|revoke|remove|rotate`.
package lifecycle

// Purpose: the codes and the typed error of the node lifecycle (Epic D15, P7-NODE-05).
// Inputs:  none.
// Outputs: E657 (node revoked or not authorized for the project) registered through model.Register;
//          *Error carrying a code and a stable reason key.
// Constraints: host-key refusals reuse E655 and a missing node reuses E650 (both owned by their own
//          packages); a bad argument or a binary that does not fit the node is E651. Messages hold
//          reason text and ids only, never a key, a token or a command's stderr.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Codes the lifecycle raises.
const (
	CodeRevoked  = "E657"
	CodeHostKey  = "E655"
	CodeNotFound = "E650"
	CodeInvalid  = "E651"
	CodeNoRoute  = "E656"
)

// Reason keys carried by Error.Reason.
const (
	ReasonRevoked       = "revoked"
	ReasonNotAuthorized = "not_authorized"
	ReasonNoTTY         = "confirmation_required"
	ReasonDeclined      = "declined"
	ReasonKeyMismatch   = "host_key_mismatch"
	ReasonNoKey         = "host_key_unknown"
	ReasonPartial       = "partial_revoke"
	ReasonUsage         = "usage"
)

// Error is a lifecycle failure.
type Error struct {
	Code, Reason, Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

func newErr(code, reason, format string, a ...any) *Error {
	return &Error{Code: code, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

func usage(format string, a ...any) *Error { return newErr(CodeInvalid, ReasonUsage, format, a...) }

// CodeOf returns the code of a lifecycle error or of a store error ("E650: node not found"), or "".
func CodeOf(err error) string {
	var le *Error
	if errors.As(err, &le) {
		return le.Code
	}
	if err == nil {
		return ""
	}
	if m := err.Error(); len(m) >= 5 && m[0] == 'E' && m[4] == ':' {
		return m[:4]
	}
	return ""
}

func init() {
	model.Register(model.Code{ID: CodeRevoked, Class: "usage", Summary: "node revoked or not authorized for the project", Fix: "Run nself ci nodes show <node>; add the node again or ask an admin to recover it"})
}
