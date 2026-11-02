package exec

// Purpose: let a sandbox runtime say why a container died when the job could not (D-1580 fork bomb).
// Inputs:  the planner of the attempt, the run, the instant it began, the exit code and the kernel counter.
// Outputs: the reason of a failed run ("resource_limit" or ""), with the evidence written to the job output.
// Constraints: output and exit code alone cannot prove a process limit. A script that just runs `exit 2`
// keeps its class. Two signals decide: the container cgroup's pids.events `max` counter (the kernel's own
// count of forks refused at the limit) and the runtime's own record (gVisor's debug log). When neither
// fires, the readings of both are written so the next flake explains itself.

import "time"

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

// sandboxFailure asks the planner for the sandbox's own account of a failed run.
func sandboxFailure(plan ContainerPlanner, run ContainerRun, began time.Time, code int) string {
	if code == 0 {
		return ""
	}
	if r, ok := plan.(SandboxFailureReader); ok {
		return r.SandboxFailure(run, began)
	}
	return ""
}

// sandboxReason classifies a failed run from the kernel's counter, then the runtime's record, and
// writes the evidence (or, on a miss, every reading) to the job output.
func (e *Executor) sandboxReason(s JobSpec, run ContainerRun, began time.Time, code int, pids pidsReading) string {
	say := func(text string) {
		if s.Output != nil {
			s.Output([]byte("nself-ci: " + text + "\n"))
		}
	}
	if code == 0 {
		return ""
	}
	if pids.Hit() {
		say("the container's process limit refused forks: " + pids.String())
		return "resource_limit"
	}
	line := sandboxFailure(s.Plan, run, began, code)
	if line != "" {
		say("the sandbox died from the container's process limit: " + line)
		return "resource_limit"
	}
	note := "no runtime record"
	if n, ok := s.Plan.(SandboxNoter); ok {
		note = n.SandboxNote(run, began)
	}
	say("no process limit evidence; " + pids.String() + "; " + note)
	return ""
}
