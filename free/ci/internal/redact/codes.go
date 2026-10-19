package redact

// Purpose: the package's error codes (E673 below the minimum length, E680 above the maximum; TRUST Epic D25) and the error type.
// Inputs:  none. Outputs: Error, MinValueLen, MaxValueLen, model.Register of E673 and E680.
// Constraints: an error string never contains a registered value or any part of one.

import "github.com/nself-org/plugins/free/ci/internal/model"

// MinValueLen is the shortest value Register accepts. Shorter values cannot be
// masked without destroying ordinary output, and they are refused, never skipped.
const MinValueLen = 6

// MaxValueLen is the longest value Register accepts (128 KiB). The cache scan
// (cache/specs scanOverlap, 1 MiB) finds a value across a window edge only when
// its longest encoded form fits the overlap, and the widest form (every byte as
// a \uXXXX escape) is 6 times the value, so 8 times the cap must stay within it.
const MaxValueLen = 128 << 10

// Error carries a registered machine-readable code.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func init() {
	model.Register(model.Code{ID: "E673", Class: "usage", Summary: "value below the redaction minimum length",
		Fix: "Register values of at least 6 bytes; shorter values cannot be masked"})
	model.Register(model.Code{ID: "E680", Class: "usage", Summary: "value above the redaction maximum length",
		Fix: "Register values of at most 128 KiB; a longer value cannot be found reliably in a scanned stream"})
}

// errTooShort never includes the value, only the rule.
func errTooShort() error {
	return &Error{Code: "E673", Message: "value is shorter than the 6-byte minimum; it was not registered"}
}

// errTooLong never includes the value, only the rule.
func errTooLong() error {
	return &Error{Code: "E680", Message: "value is longer than the 128 KiB maximum; it was not registered"}
}
