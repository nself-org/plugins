package netscope

// Purpose: the error type of this package and the one code it carries.
// Inputs:  none. Outputs: E679 (network scope cannot be enforced or was refused), the isolation package's code.
// Constraints: no code is registered here. The Epic assigned E680 to this package; internal/redact no longer
// uses it (D-1334), but the lead ruling keeps one code: a scope that cannot be set up is the same condition as one that cannot be enforced, so
// E679 stays the single code and its Reason names the case (lead ruling in the TRUST Epic Invalidations).

import "fmt"

// CodeScopeFailed is the code of a scope that could not be set up or was refused.
const CodeScopeFailed = "E679"

// Error is a coded netscope failure.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
