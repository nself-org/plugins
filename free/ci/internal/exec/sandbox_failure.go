package exec

// Purpose: let a sandbox runtime say why a container died when the job could not (D-1580 fork bomb).
// Inputs:  the planner of the attempt, the run, the instant it began, the exit code and the job output tail.
// Outputs: the reason of a silent failed run ("resource_limit" or ""), with the evidence written to the job output.
// Constraints: only a failed run that printed nothing is asked, so a job that exits non-zero with output, or
// a script that just runs `exit 2` (the kernel counts no refused fork, the runtime records no death for it),
// keeps its class. Two signals decide: the container cgroup's pids.events `max` counter (the kernel's own
// count of forks refused at the limit) and the runtime's own record (gVisor's debug log). When neither
// fires, the readings of both are written so the next flake explains itself.

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

// SandboxNoter is implemented by a planner that can say what its runtime record held when it showed no death.
type SandboxNoter interface {
	SandboxNote(run ContainerRun, began time.Time) string
}

// silentFailure reports a failed run that printed nothing.
func silentFailure(code int, tail string) bool { return code != 0 && strings.TrimSpace(tail) == "" }

// sandboxFailure asks the planner for the sandbox's own account of a silent failed run.
func sandboxFailure(plan ContainerPlanner, run ContainerRun, began time.Time, code int, tail string) string {
	if !silentFailure(code, tail) {
		return ""
	}
	if r, ok := plan.(SandboxFailureReader); ok {
		return r.SandboxFailure(run, began)
	}
	return ""
}

// silentReason classifies a silent failed run from the kernel's counter, then the runtime's record, and
// writes the evidence (or, on a miss, every reading) to the job output.
func (e *Executor) silentReason(s JobSpec, run ContainerRun, began time.Time, code int, tail string, pids pidsReading) string {
	say := func(text string) {
		if s.Output != nil {
			s.Output([]byte("nself-ci: " + text + "\n"))
		}
	}
	line := sandboxFailure(s.Plan, run, began, code, tail)
	switch {
	case pids.Hit():
		say("the container's process limit refused forks: " + pids.String())
		return "resource_limit"
	case line != "":
		say("the sandbox died from the container's process limit: " + line)
		return "resource_limit"
	}
	note := "no runtime record"
	if n, ok := s.Plan.(SandboxNoter); ok {
		note = n.SandboxNote(run, began)
	}
	say("silent failure, not a process limit death; " + pids.String() + "; " + note)
	return ""
}
