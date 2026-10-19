package netscope

// Purpose: the one error code of this package and the error type that carries it.
// Inputs:  none. Outputs: E680 (network scope setup failed), registered through model.Register.
// Constraints: only E680 belongs here (Epic D25); E679 (scope unenforceable) is the isolation package's and is
// reused through isolation.ScopeUnenforceable so there is one meaning per code.

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeScopeFailed is the code of a scope that could not be set up or was refused.
const CodeScopeFailed = "E680"

func init() {
	model.Register(model.Code{ID: CodeScopeFailed, Class: "infra", Summary: "network scope setup failed", Fix: "Check that the container runtime can create an internal network and start the proxy sidecar, or declare a scope the job's trust class may use"})
}

// Error is a coded netscope failure.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
