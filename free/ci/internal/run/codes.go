package run

// Purpose: the error codes of the run orchestrator (Epic D14, E624-E627) and
// the typed error that carries one.
// E624 --sha is not a V15 option; E625 --trust owner without a registered
// classifier; E626 unknown or unusable run (ci runs, cancel); E627 an option
// or hook the orchestrator cannot honour.
// Constraints: registered through model.Register, never a panic.

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// The codes this package raises.
const (
	CodeSHA      = "E624"
	CodeTrust    = "E625"
	CodeRun      = "E626"
	CodeOptions  = "E627"
	codeNoPlace  = "E618" // plan.CodeUnplaceable
	codeStoreRef = "E605"
)

func init() {
	for _, c := range []model.Code{
		{ID: CodeSHA, Class: "usage", Summary: "--sha is not supported in V15", Fix: "Run `nself ci evidence verify <rev>` to check a revision"},
		{ID: CodeTrust, Class: "usage", Summary: "--trust needs a registered trust classifier", Fix: "Run without --trust, or install the trust component"},
		{ID: CodeRun, Class: "usage", Summary: "unknown or unusable run", Fix: "List runs with `nself ci runs` and use a run id from the list"},
		{ID: CodeOptions, Class: "usage", Summary: "run option not available", Fix: "Check `nself ci --help` for the options this build supports"},
	} {
		model.Register(c)
	}
}

// Error is a run failure with its code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, a ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, a...)}
}

var codeRe = regexp.MustCompile(`^(E\d{3})\b`)

// CodeOf returns the E-code an error carries ("" when none). Every package of
// the fabric formats its errors "E###: message".
func CodeOf(err error) string {
	var re *Error
	if errors.As(err, &re) {
		return re.Code
	}
	for e := err; e != nil; e = errors.Unwrap(e) {
		if m := codeRe.FindStringSubmatch(e.Error()); m != nil {
			return m[1]
		}
	}
	return ""
}
