package local

// Purpose: the one error code of this package (Epic D17): E698, the local ephemeral
// provider is unavailable (no container runtime, no Tart, an adapter that is not
// registered, a spec for another platform). It is shared with the cloud VM provider
// (P7-SCHED-10), which registers nothing new.
// Inputs: a stable reason and a detail. Outputs: *Error, whose text starts with the code.
// Constraints: registered through model.Register, never a panic; a detail holds
// reasons and counts, never a path to a secret or a token.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeUnavailable is E698.
const CodeUnavailable = "E698"

// Stable reasons carried by an E698.
const (
	ReasonNoRuntime  = "ephemeral.no_runtime"  // Linux host with no container runtime
	ReasonNoTart     = "ephemeral.no_tart"     // macOS host without a usable Tart
	ReasonNoAdapter  = "ephemeral.no_adapter"  // the isolation profile is not registered
	ReasonPlatform   = "ephemeral.platform"    // the spec is for another OS or architecture
	ReasonBadRequest = "ephemeral.bad_request" // an intent that breaks the provider contract
	ReasonUnsupport  = "ephemeral.unsupported" // a host OS the provider has no sandbox for
	ReasonBackend    = "ephemeral.backend"     // the runtime or the adapter failed
)

func init() {
	model.Register(model.Code{ID: CodeUnavailable, Class: "infra", Summary: "the ephemeral provider is unavailable: missing tool, adapter, token file or agent artifact",
		Fix: "Install a container runtime (Linux) or Tart (Apple silicon), or read the reason in `nself ci doctor`; no slot is offered below container isolation"})
}

// Error is an unavailable ephemeral provider.
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

// Is makes every E698 a burst.ErrUnavailable: the lifecycle retries it later and never counts it as a quota refusal.
func (e *Error) Is(target error) bool { return target == burst.ErrUnavailable }

func unavailable(reason string, cause error, format string, a ...any) *Error {
	return &Error{Code: CodeUnavailable, Reason: reason, Detail: fmt.Sprintf(format, a...), cause: cause}
}

// CodeOf returns the code of err ("" when it is not an *Error of this package).
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ReasonOf returns the reason of err ("" when it is not an *Error of this package).
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
