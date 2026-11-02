package exec

// Purpose: let a sandbox runtime say why a container died when the job could not (D-1580 fork bomb).
// Inputs:  the planner, run, start time, exit code, kernel counter and full-output eligibility.
// Outputs: the reason of a failed run ("resource_limit" or ""), with the evidence written to the job output.
// Constraints: only a silent failure or exact task-limit reports are eligible. Any other job output
// keeps its class even if a fork was refused earlier, so a real test failure cannot become infra.
// A script that just runs `exit 2` also keeps its class without evidence. Signals include a bounded
// in-sandbox probe reporting EAGAIN at exactly the configured task limit,
// the container cgroup's pids.events `max` counter (the kernel's own
// count of host forks refused at the limit) and the runtime's own record (gVisor's debug log).
// Without evidence, the readings are written so the next failure explains itself.

import (
	"strings"
	"time"
)

// sandboxOutput checks every output byte with bounded storage, including bytes evicted from the tail.
// Only the complete report below is allowed; prefixes with appended job text are rejected.
type sandboxOutput struct {
	line      string
	jobOutput bool
	nonSilent bool
	reported  bool
}

// SandboxTaskLimitReport is emitted by the bounded integration probe only after checking
// RLIMIT_NPROC, live task count and clone's errno. Like a shell's "can't fork", job output
// is diagnostic evidence, not attestation against a job deliberately forging diagnostics.
// Enforcement: runsc release-20261012.0 pkg/sentry/kernel/kernel.go:96-103
// returns EAGAIN above RLIMIT_NPROC; task_start.go:155 checks every new task.
const SandboxTaskLimitReport = "nself-ci: task limit reached: limit=512 tasks=512 errno=EAGAIN"

func (o *sandboxOutput) write(b []byte) {
	o.nonSilent = o.nonSilent || strings.TrimSpace(string(b)) != ""
	for _, c := range b {
		if o.jobOutput {
			return
		}
		if c == '\n' {
			o.jobOutput = strings.TrimSpace(o.line) != "" && o.line != SandboxTaskLimitReport
			o.reported = o.reported || o.line == SandboxTaskLimitReport
			o.line = ""
		} else if len(o.line) < len(SandboxTaskLimitReport) {
			o.line += string(c)
		} else {
			o.jobOutput = true
		}
	}
}

func (o sandboxOutput) eligible() bool {
	return !o.nonSilent || (!o.jobOutput && (strings.TrimSpace(o.line) == "" || o.line == SandboxTaskLimitReport))
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
	if output.reported || output.line == SandboxTaskLimitReport {
		return "resource_limit"
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
