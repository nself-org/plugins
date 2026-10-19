package backup

// Purpose: E668 "coordinator backup or restore refused" and its reason keys (Epic D15), the one error type of
//          this package.
// Inputs:  none.
// Outputs: *Error with a stable reason key; the code registered through model.Register; ReasonOf.
// Constraints: a message holds the reason and file names only, never a key, a KEK byte or a certificate body;
//          a restore raises every verification reason before it writes anything to the data dir.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeBackup is E668.
const CodeBackup = "E668"

// Reason keys carried by Error.Reason.
const (
	ReasonMissingKEK         = "missing_kek"
	ReasonVersion            = "version"
	ReasonChecksum           = "checksum"
	ReasonMAC                = "mac"
	ReasonRunningCoordinator = "running_coordinator"
	ReasonFormat             = "format"
	ReasonNoState            = "no_state"
	ReasonTooLarge           = "too_large"
	ReasonIO                 = "io"
	ReasonRotate             = "rotate"
)

func init() {
	model.Register(model.Code{ID: CodeBackup, Class: "infra", Summary: "coordinator backup or restore refused",
		Fix: "Check the reason: pass the KEK of the coordinator that made the backup (--kek), stop the coordinator before `serve restore`, or take a new backup"})
}

// Error is an E668 failure.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s (%s)", CodeBackup, e.Msg, e.Reason) }

// ErrCode and ErrReason let a command map the error to its JSON body.
func (e *Error) ErrCode() string   { return CodeBackup }
func (e *Error) ErrReason() string { return e.Reason }

func fail(reason, format string, a ...any) error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the E668 reason key of err, or "" when err is not an E668 error.
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}
