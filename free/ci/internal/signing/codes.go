package signing

// Purpose: the error code of the release signer (Epic CACHE D20: E717
// "signing failed") and its reason keys.
// Inputs:  none (registration at init).
// Outputs: Error, Failf, IsReason and the Reason constants.
// Constraints: an error text never carries key material, a payload or a store
// path; it names the reason and the field that failed, at most 64 printable
// bytes of any input echoed. Nothing is signed when any error is returned.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeSigning is E717.
const CodeSigning = "E717"

// Reason keys of E717.
const (
	ReasonKeyMissing         = "key_missing"
	ReasonKeyInvalid         = "key_invalid"
	ReasonKeyIDMismatch      = "key_id_mismatch"
	ReasonPayloadRejected    = "payload_rejected"
	ReasonDigestNotCommitted = "digest_not_committed"
	ReasonSignerFailed       = "signer_failed"
	ReasonCLITooOld          = "cli_too_old"
	ReasonSecretExists       = "secret_exists"
)

// Error is a signing refusal with a stable code and reason.
type Error struct {
	Reason string
	Msg    string
}

func (e *Error) Error() string { return CodeSigning + " " + e.Reason + ": " + e.Msg }

// Code returns the E-code.
func (e *Error) Code() string { return CodeSigning }

// Is matches on the reason when the target names one.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && (t.Reason == "" || t.Reason == e.Reason)
}

// Failf builds an E717 refusal.
func Failf(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// IsReason reports whether err is an E717 refusal with the given reason.
func IsReason(err error, reason string) bool {
	var e *Error
	return errors.As(err, &e) && e.Reason == reason
}

// echo bounds how much of an input an error repeats.
func echo(s string) string { return clip(s, 64) }

// clip is echo with a chosen bound.
func clip(s string, n int) string {
	b := make([]byte, 0, n+3)
	for i := 0; i < len(s); i++ {
		if len(b) >= n {
			return string(append(b, "..."...))
		}
		c := s[i]
		if c < 0x20 || c > 0x7e {
			c = '?'
		}
		b = append(b, c)
	}
	return string(b)
}

func init() {
	model.Register(model.Code{ID: CodeSigning, Class: "infra", Summary: "release signing failed",
		Fix: "Run `nself ci artifacts key init` once to create the release key, make sure every attempt of the run is finished, and release only digests that are committed records of the run"})
}
