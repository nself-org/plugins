package redact

// Purpose: the package's one error code (E673, TRUST Epic D25) and its error type.
// Inputs:  none. Outputs: Error, MinValueLen, model.Register of E673.
// Constraints: an error string never contains a registered value or any part of one.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// MinValueLen is the shortest value Register accepts. Shorter values cannot be
// masked without destroying ordinary output, and they are refused, never skipped.
const MinValueLen = 6

// MaxValueLen is the longest value Register accepts. The scanners keep an overlap
// between their windows (artifacts/records/scan.go 4 MiB, cache/specs/scan.go
// 1 MiB) and find a value only if its longest encoded form fits in the smaller
// one; the longest form is MaxFormExpansion times the value (a CRLF line end
// written as JSON \uXXXX escapes). A longer value is refused, never skipped.
const MaxValueLen = 64 << 10

// MaxFormExpansion is the most one registered byte can grow to in any form of
// encodings.go. A test over adversarial values keeps it true.
const MaxFormExpansion = 12

// Error carries a registered machine-readable code.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func init() {
	model.Register(model.Code{ID: "E673", Class: "usage", Summary: "value outside the redaction length bounds",
		Fix: "Register values of 6 bytes up to the documented maximum; shorter values cannot be masked, longer ones cannot be scanned for in stored artifacts"})
}

// errTooLong never includes the value, only the rule.
func errTooLong() error {
	return &Error{Code: "E673", Message: fmt.Sprintf("value is longer than the %d-byte maximum; it was not registered", MaxValueLen)}
}

// errTooShort never includes the value, only the rule.
func errTooShort() error {
	return &Error{Code: "E673", Message: "value is shorter than the 6-byte minimum; it was not registered"}
}
