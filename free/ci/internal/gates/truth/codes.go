package truth

// Purpose: the error code of the truth check (Epic CI D14: P7-CI-08 owns E644).
// Inputs:  none (registration at init).
// Outputs: Error, Failf.
// Constraints: one code, E644, for every refusal of a rule file; the message
// always names the file and, where there is one, the key, so the author can fix
// the line without opening the engine.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeTruth is E644.
const CodeTruth = "E644"

func init() {
	model.Register(model.Code{ID: CodeTruth, Class: "code", Summary: "truth rule file is invalid or unreadable",
		Fix: "Correct the key named in the message in the rule file and retry"})
}

// Error is a refused rule file: File is repo-relative, Key the offending key.
type Error struct {
	File, Key, Msg string
}

func (e *Error) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("%s %s: %s", CodeTruth, e.File, e.Msg)
	}
	return fmt.Sprintf("%s %s: %s: %s", CodeTruth, e.File, e.Key, e.Msg)
}

// Code returns the E-code.
func (e *Error) Code() string { return CodeTruth }

func failf(file, key, format string, a ...any) error {
	return &Error{File: file, Key: key, Msg: fmt.Sprintf(format, a...)}
}
