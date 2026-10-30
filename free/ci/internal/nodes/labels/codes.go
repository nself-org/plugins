// Package labels validates operator-assigned node labels and feeds them to placement.
package labels

// Purpose: the code and typed error of node label rejections (P7-NODE-30).
// Inputs:  none.
// Outputs: E658 registered through model.Register; *Error carrying the code and a stable reason key.
// Constraints: one code for every label rejection; the reason key tells syntax, reserved prefix and
//          count limit apart.

import (
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeLabel is the code every label rejection carries.
const CodeLabel = "E658"

// Reason keys carried by Error.Reason.
const (
	ReasonSyntax   = "label_syntax"
	ReasonReserved = "label_reserved"
	ReasonTooMany  = "too_many"
	// ReasonTeamWildcard refuses `projects add --all` against a TEAM coordinator.
	ReasonTeamWildcard = "team_wildcard"
	// ReasonProjectUnknown is a project argument that names no repository id.
	ReasonProjectUnknown = "project_unknown"
)

func init() {
	model.Register(model.Code{ID: CodeLabel, Class: "usage", Summary: "invalid node label", Fix: "Use lowercase keys, plain short values, at most 32 labels and no nself. prefix"})
}

// Error is a label rejection.
type Error struct {
	Code, Reason, Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

func newErr(reason, format string, a ...any) *Error {
	return &Error{Code: CodeLabel, Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// ReasonOf returns the reason key of a label error, or "".
func ReasonOf(err error) string {
	var le *Error
	if errors.As(err, &le) {
		return le.Reason
	}
	return ""
}
