package slots

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// CodeFull is E654: the node has no free slot (Epic D15, X10).
const CodeFull = "E654"

func init() {
	model.Register(model.Code{ID: CodeFull, Class: "infra", Summary: "node slots full", Fix: "Wait for a running job to finish or raise the agent's slot count"})
}

// Error carries E654 and the reason: slots_full, or os_unsupported where the
// host has no file lock (the windows stub).
type Error struct{ Reason string }

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", CodeFull, e.Reason) }
