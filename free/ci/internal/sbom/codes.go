package sbom

// Purpose: the error code of the SBOM step (Epic CACHE D20: E718 "SBOM failed").
// Inputs:  none (registration at init).
// Outputs: Error, IsReason, the reason keys.
// Constraints: every refusal carries one reason key; the Fix text names the
// install command so a job that lacks the pinned syft is told how to get it.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeSBOM is E718.
const CodeSBOM = "E718"

// Reason keys of E718.
const (
	ReasonToolMissing  = "tool_missing"
	ReasonToolVersion  = "tool_version"
	ReasonToolChecksum = "tool_checksum"
	ReasonInvalid      = "invalid"
)

// InstallCommand is what the E718 remediation names.
const InstallCommand = "nself ci artifacts tools install"

// Error is an SBOM refusal with a stable code and reason.
type Error struct {
	Reason string
	Msg    string
}

func (e *Error) Error() string { return CodeSBOM + " " + e.Reason + ": " + e.Msg }

// Code returns the E-code.
func (e *Error) Code() string { return CodeSBOM }

// Is matches on the reason when the target names one.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && (t.Reason == "" || t.Reason == e.Reason)
}

// Failf builds an E718 refusal.
func Failf(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// IsReason reports whether err is an SBOM refusal with the given reason.
func IsReason(err error, reason string) bool {
	var e *Error
	return errors.As(err, &e) && e.Reason == reason
}

// ReasonOf returns the reason key of an SBOM refusal ("" for any other error).
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

func init() {
	model.Register(model.Code{ID: CodeSBOM, Class: "infra", Summary: "SBOM failed",
		Fix: "Run `" + InstallCommand + "` to place the pinned syft, then retry; an invalid SBOM means syft's output failed the structural check, so rerun with the pinned version"})
}
