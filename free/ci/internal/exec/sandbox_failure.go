package exec

// Purpose: let a sandbox runtime say why a container died when the job could not (D-1580 fork bomb).
// Inputs:  the planner of the attempt, the run, the instant it began, the exit code and the job output tail.
// Outputs: the runtime's own line about the death, or "" when it has none.
// Constraints: only a failed run that printed nothing is asked, so a job that exits non-zero with output, or
// a script that just runs `exit 2` (the runtime records no death for it), keeps its class.

import (
	"strings"
	"time"
)

// SandboxFailureReader is implemented by a planner whose runtime keeps a record of a sandbox that died
// (gVisor's debug log). SandboxFailure returns the runtime's line when the sandbox of run died from the
// container's process limit, else "".
type SandboxFailureReader interface {
	SandboxFailure(run ContainerRun, began time.Time) string
}

// sandboxFailure asks the planner for the sandbox's own account of a silent failed run.
func sandboxFailure(plan ContainerPlanner, run ContainerRun, began time.Time, code int, tail string) string {
	if code == 0 || strings.TrimSpace(tail) != "" {
		return ""
	}
	if r, ok := plan.(SandboxFailureReader); ok {
		return r.SandboxFailure(run, began)
	}
	return ""
}
