package roles

// Purpose: E687 "not authorized" and its reason keys (Epic D25), the one error type of this package and
// the code internal/audit carries for a refused event.
// Inputs:  none.
// Outputs: *Error with a stable reason key; the code registered through model.Register.
// Constraints: a message holds the reason and ids only, never a token, a certificate body or a secret.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeNotAuthorized is E687.
const CodeNotAuthorized = "E687"

// Reason keys carried by Error.Reason.
const (
	ReasonRole         = "role"
	ReasonProject      = "project"
	ReasonNoGrant      = "no_grant"
	ReasonNodeKind     = "node_kind"
	ReasonBadCert      = "bad_certificate"
	ReasonTokenUnknown = "token_unknown"
	ReasonTokenExpired = "token_expired"
	ReasonTokenRevoked = "token_revoked"
	ReasonSelfApproval = "self_approval"
	ReasonNoAuthor     = "author_unknown"
	ReasonUnknownRole  = "unknown_role"
	ReasonNoActor      = "no_actor"
	ReasonUnknownAct   = "unknown_action"
	ReasonBadRequest   = "bad_request"
)

func init() {
	model.Register(model.Code{ID: CodeNotAuthorized, Class: "usage", Summary: "not authorized",
		Fix: "Use an identity that holds the needed role (`nself ci serve admin roles list`), or ask an admin to grant it"})
}

// Error is an E687 failure.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s (%s)", CodeNotAuthorized, e.Msg, e.Reason) }

// ErrCode and ErrReason let a route handler map the error to its JSON body.
func (e *Error) ErrCode() string   { return CodeNotAuthorized }
func (e *Error) ErrReason() string { return e.Reason }

func fail(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the E687 reason key of err, or "" when err is not an E687 error.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
