package exec

// Purpose: let a sandbox runtime say why a container died when the job could not (D-1580 fork bomb).
// Inputs:  the planner, run, start time, exit code, kernel counter and full-output eligibility.
// Outputs: the reason of a failed run ("resource_limit" or ""), with the evidence written to the job output.
// Constraints: only a silent failure or exact runtime diagnostic lines are eligible. Any job output
// keeps its class even if a fork was refused earlier, so a real test failure cannot become infra.
// A script that just runs `exit 2` also keeps its class without evidence. Two signals decide:
// the container cgroup's pids.events `max` counter (the kernel's own
// count of forks refused at the limit) and the runtime's own record (gVisor's debug log). When neither
// fires, the readings of both are written so the next flake explains itself.

import (
	"strings"
	"time"
)

// sandboxOutput checks every output byte with bounded storage, including bytes evicted from the tail.
// Only the complete diagnostic below is allowed; prefixes with appended job text are rejected.
type sandboxOutput struct {
	line      string
	jobOutput bool
	nonSilent bool
}

const sandboxWaitDiagnostic = "sandbox wait failed"

func (o *sandboxOutput) write(b []byte) {
	o.nonSilent = o.nonSilent || strings.TrimSpace(string(b)) != ""
	for _, c := range b {
		if o.jobOutput {
			return
		}
		if c == '\n' {
			o.jobOutput = o.line != "" && o.line != sandboxWaitDiagnostic
			o.line = ""
		} else if len(o.line) < len(sandboxWaitDiagnostic) {
			o.line += string(c)
		} else {
			o.jobOutput = true
		}
	}
}

func (o sandboxOutput) eligible() bool {
	return !o.nonSilent || (!o.jobOutput && (o.line == "" || o.line == sandboxWaitDiagnostic))
}

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
func (e *Executor) sandboxReason(s JobSpec, run ContainerRun, began time.Time, code int, pids pidsReading, output sandboxOutput) string {
	say := func(text string) {
		if s.Output != nil {
			s.Output([]byte("nself-ci: " + text + "\n"))
		}
	}
	if code == 0 || !output.eligible() {
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
