package cloudvm

// Purpose: the one error code of this package (Epic D17): E698, a cloud VM provider
// call that cannot start or cannot be trusted (missing infra flag, bad token file,
// bad recipe, bad answer from the infra CLI).
// Inputs: a reason and a detail. Outputs: *Error, whose text starts with the code.
// Constraints: E698 is registered by P7-SCHED-09 (internal/providers/ephemeral/local, the
// package that owns the code), so this package declares the ID only and has no codes.go: a second
// model.Register of E698 would be a duplicate in model.CodeProblems once both link into cmd.
// A detail holds reasons, ids and exit codes, never a token, a pairing code or user data.
// Failures the burst lifecycle classifies (not found, unavailable, quota) are returned
// as the burst sentinels, wrapped, so errors.Is works.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/burst"
)

// CodeProviderFailed is E698.
const CodeProviderFailed = "E698"

// Reasons of an E698 from this package.
const (
	ReasonRecipe       = "recipe"        // a recipe key is missing or a value is not usable
	ReasonTokenFile    = "token_file"    // the token file is missing, not regular, wider than 0600 or not ours
	ReasonNoUserData   = "no_user_data"  // the installed nself has no --user-data-file
	ReasonNoNself      = "no_nself"      // the nself binary cannot be found
	ReasonPlatform     = "platform"      // the spec's platform does not match the server type
	ReasonAgent        = "agent"         // no pinned agent for the VM's arch
	ReasonInfra        = "infra"         // the infra CLI failed
	ReasonAnswer       = "answer"        // the infra CLI answered something unusable
	ReasonEnroll       = "enroll"        // the pairing offer failed
	ReasonUserData     = "user_data"     // the user data cannot be rendered or written
	ReasonTokenRefused = "token_refused" // the provider rejected the token
	ReasonEstimate     = "estimate"      // no usable declared rate
	ReasonUnsupported  = "unsupported"   // this OS cannot keep the token file private
)

// Error is a failed cloud VM provider call.
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

func fail(reason string, cause error, format string, a ...any) *Error {
	return &Error{Code: CodeProviderFailed, Reason: reason, Detail: fmt.Sprintf(format, a...), cause: cause}
}

// ReasonOf returns the reason of an E698 from this package ("" when it is not one).
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

// CodeOf returns the code of an error from this package ("" when it is not one).
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// The lifecycle sentinels, re-exported for readers of this package.
var (
	errNotFound    = burst.ErrNotFound
	errUnavailable = burst.ErrUnavailable
	errQuota       = burst.ErrQuota
)
