// Package update is the node upgrade state machine behind `nself ci nodes upgrade` and the agent's own `upgrade` subcommand.
package update

// Purpose: the code and the typed error of an agent upgrade (hq NODE D15, P7-NODE-24).
// Inputs:  none.
// Outputs: E666 registered through model.Register; *Error carrying the code and a stable reason key.
// Constraints: one code, E666, with three reason keys: ceiling (the target version is past what a registered coordinator
//          accepts), downgrade (a lower version without --allow-downgrade), running_job (the node still holds a job).
//          Messages hold node ids, versions and protocol numbers only, never a key, a token or command output.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeRefused is the code of every refused upgrade.
const CodeRefused = "E666"

// Reason keys carried by Error.Reason.
const (
	ReasonCeiling    = "ceiling"
	ReasonDowngrade  = "downgrade"
	ReasonRunningJob = "running_job"
)

// Error is a refused upgrade.
type Error struct{ Reason, Msg string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", CodeRefused, e.Msg) }

func refuse(reason, format string, a ...any) *Error {
	return &Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the reason key of a refused upgrade, or "".
func ReasonOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

func init() {
	model.Register(model.Code{ID: CodeRefused, Class: "usage", Summary: "agent upgrade refused (protocol ceiling, downgrade or running job)",
		Fix: "Run nself ci nodes show <node>; wait for the job, pass --allow-downgrade, or upgrade the coordinator first"})
}
