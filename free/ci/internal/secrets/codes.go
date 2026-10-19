package secrets

// Purpose: the package's error codes (TRUST Epic D25, E674-E677) and the one error type.
// Inputs:  none. Outputs: Error, the four constructors, model.Register of the codes.
// Constraints: an error string never contains a secret value, any part of one, or the text of
// a store file; messages name refs, reasons and nodes only.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Reasons carried by an E677 error (the trust package's own reasons are used for policy refusals).
const (
	ReasonNodeNotEligible = "secrets.node_not_eligible"
	ReasonNodeNoRecipient = "secrets.node_no_recipient"
	ReasonRefsOnly        = "secrets.coordinator_refs_only"
	ReasonLegacyScope     = "secrets.legacy_scope"
)

// Error carries a registered machine-readable code and, for E677, the refusal reasons.
type Error struct {
	Code, Message string
	Reasons       []string
	// Cause is the CLI's own error code (E54x) behind an E676, "" when there is none.
	Cause string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func init() {
	for _, c := range []model.Code{
		{ID: "E674", Class: "usage", Summary: "invalid secret reference or environment-name collision", Fix: "Write refs as project/NAME, team/NAME, release/NAME, environment/<env>/NAME or deploy/<env>/NAME, and give each NAME once"},
		{ID: "E675", Class: "infra", Summary: "secret resolver unavailable", Fix: "Install an nself CLI that has `nself config secrets resolve`, or remove the secrets from the job"},
		{ID: "E676", Class: "infra", Summary: "secret resolution failed", Fix: "Check that the ref exists in the store file and that this node's age identity was granted the file; the attempt did not start"},
		{ID: "E677", Class: "usage", Summary: "secret class or node not eligible", Fix: "Run the job on a node and trust class the policy allows for this secret class, or grant the node (nself ci nodes secrets grant) if it is eligible"},
	} {
		model.Register(c)
	}
}

func newErr(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// errRef is E674: the text names the rule, never the offending input beyond a bounded echo.
func errRef(format string, args ...any) error { return newErr("E674", format, args...) }

// errUnavailable is E675.
func errUnavailable(format string, args ...any) error { return newErr("E675", format, args...) }

// errResolve is E676.
func errResolve(format string, args ...any) error { return newErr("E676", format, args...) }

// errNotEligible is E677 with its reasons.
func errNotEligible(reasons []string, format string, args ...any) error {
	e := newErr("E677", format, args...)
	e.Reasons = append([]string(nil), reasons...)
	return e
}

// CodeOf returns the E-code of an error from this package, or "".
func CodeOf(err error) string {
	for err != nil {
		if e, ok := err.(*Error); ok {
			return e.Code
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return ""
		}
		err = u.Unwrap()
	}
	return ""
}
