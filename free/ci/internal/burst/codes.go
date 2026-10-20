package burst

// Purpose: the two error codes of this package (Epic D17): E696, a provision that
// is refused or failed (cap, breaker, provider error), and E697, a teardown that
// failed (alerted once, still retried at every sweep).
// Inputs: a reason and a detail. Outputs: *Error, whose text starts with the code.
// Constraints: registered through model.Register, never a panic; a detail holds
// ids, counts and reasons, never a token, a rate or a secret.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes.
const (
	CodeProvisionRefused = "E696"
	CodeTeardownFailed   = "E697"
)

func init() {
	model.Register(model.Code{ID: CodeProvisionRefused, Class: "usage", Summary: "burst provisioning refused or failed: cap, breaker, budget or provider error",
		Fix: "Read the reason in `nself ci explain`; raise burst.max_concurrent, wait for the breaker to close, or fix the provider"})
	model.Register(model.Code{ID: CodeTeardownFailed, Class: "internal", Summary: "burst teardown failed; it is alerted once and retried at every sweep until the resource is gone",
		Fix: "Check the provider API and credentials; the sweeper keeps retrying and new provisioning on that provider stays blocked until the resource is gone"})
}

// Error is a refused or failed burst operation.
type Error struct {
	Code, Reason, Detail string
	cause                error
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Code + ": " + e.Reason
	}
	return e.Code + ": " + e.Reason + ": " + e.Detail
}

func (e *Error) Unwrap() error { return e.cause }

func refused(reason string, cause error, format string, a ...any) *Error {
	return &Error{Code: CodeProvisionRefused, Reason: reason, Detail: fmt.Sprintf(format, a...), cause: cause}
}

func teardownFailed(cause error, format string, a ...any) *Error {
	return &Error{Code: CodeTeardownFailed, Reason: "teardown.failed", Detail: fmt.Sprintf(format, a...), cause: cause}
}

// CodeOf returns the burst code of err ("" when it is not one).
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ReasonOf returns the reason of a burst error ("" when it is not one).
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
