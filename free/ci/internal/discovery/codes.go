package discovery

// Purpose: E667 "LAN discovery is disabled" (Epic D15) and its reason keys, the error type of this package.
// Inputs:  none.
// Outputs: *Error with a stable reason key; the code registered through model.Register.
// Constraints: a message holds reason keys only, never an address, a host name or a user name.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeDisabled is E667.
const CodeDisabled = "E667"

// Reason keys carried by Error.Reason.
const (
	ReasonEnvOff        = "env_off"        // NSELF_CI_DISCOVERY=0
	ReasonConfigOff     = "config_off"     // ci.discovery: off in the user config
	ReasonConfigInvalid = "config_invalid" // the user config cannot be read: discovery stays off
)

func init() {
	model.Register(model.Code{ID: CodeDisabled, Class: "usage", Summary: "LAN discovery is disabled",
		Fix: "Unset NSELF_CI_DISCOVERY=0 and remove `ci.discovery: off` from the user config.yaml to use discovery, or pair by address with `nself ci nodes pair`"})
}

// Error is an E667 failure.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string     { return fmt.Sprintf("%s: %s (%s)", CodeDisabled, e.Msg, e.Reason) }
func (e *Error) ErrCode() string   { return CodeDisabled }
func (e *Error) ErrReason() string { return e.Reason }

func disabled(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the E667 reason key of err, or "" when err is not an E667 error.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
