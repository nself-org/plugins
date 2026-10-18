package redact

// Purpose: the package's one error code (E673, TRUST Epic D25) and its error type.
// Inputs:  none. Outputs: Error, MinValueLen, model.Register of E673.
// Constraints: an error string never contains a registered value or any part of one.

import "github.com/nself-org/plugins/free/ci/internal/model"

// MinValueLen is the shortest value Register accepts. Shorter values cannot be
// masked without destroying ordinary output, and they are refused, never skipped.
const MinValueLen = 6

// Error carries a registered machine-readable code.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func init() {
	model.Register(model.Code{ID: "E673", Class: "usage", Summary: "value below the redaction minimum length",
		Fix: "Register values of at least 6 bytes; shorter values cannot be masked"})
}

// errTooShort never includes the value, only the rule.
func errTooShort() error {
	return &Error{Code: "E673", Message: "value is shorter than the 6-byte minimum; it was not registered"}
}
