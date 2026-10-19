package contention

// Purpose: run leased jobs at background priority and inside the operator's reservation, through the
// executor's Isolator hook (P7-CI-32).
// Inputs:  the inner Isolator, the current Share, a Platform.
// Outputs: the JobSpec with an argv prefix; a Priority report per job saying what was applied.
// Constraints: only an argv prefix is added (no os/exec here). Linux: /usr/bin/nice -n 10, plus a
//          /bin/sh stub that joins the attempt's cgroup when the limits can be set. macOS:
//          /usr/sbin/taskpolicy -b. A container job is not touched (host priority does not reach
//          it). Anything not applied is reported "unknown" with a reason, never guessed.

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

// sweepAge is how old an empty attempt group must be before a new job removes it.
const sweepAge = 2 * time.Minute

// Priority values.
const (
	Applied = "applied"
	Off     = "off"
	NoInfo  = "unknown"
)

// Priority reports what one job got.
type Priority struct {
	// Nice is the background scheduling class (nice 10 on Linux, taskpolicy -b on macOS).
	Nice string
	// CPU and Memory are the cgroup limits: Off when nothing is reserved, NoInfo when they could not be set.
	CPU, Memory string
	// Reason says why something is NoInfo.
	Reason string
}

// Isolator wraps another exec.Isolator and adds background priority.
type Isolator struct {
	// Inner is the executor's own isolator; it runs first and is required.
	Inner exec.Isolator
	// Share returns the current reservation share; nil means no reservation.
	Share func() reservation.Share
	// Platform defaults to HostPlatform.
	Platform *Platform
	// Report receives the per-job outcome; nil drops it.
	Report func(attempt string, p Priority)
}

// Prepare runs the inner isolator, then prefixes the command.
func (i *Isolator) Prepare(ctx context.Context, s *exec.JobSpec) error {
	if i.Inner == nil {
		return errors.New("contention: inner isolator required")
	}
	if err := i.Inner.Prepare(ctx, s); err != nil {
		return err
	}
	p := i.apply(s)
	if i.Report != nil {
		i.Report(s.AttemptID, p)
	}
	return nil
}

func (i *Isolator) apply(s *exec.JobSpec) Priority {
	pl := HostPlatform()
	if i.Platform != nil {
		pl = *i.Platform
	}
	p := Priority{Nice: NoInfo, CPU: Off, Memory: Off}
	if s.Plan != nil || s.Job.Isolation == "container" {
		p.CPU, p.Memory, p.Reason = NoInfo, NoInfo, "container job: host priority does not reach the container"
		return p
	}
	var sh reservation.Share
	if i.Share != nil {
		sh = i.Share()
	}
	var prefix []string
	switch pl.GOOS {
	case "darwin":
		if tp := pl.first("/usr/sbin/taskpolicy"); tp != "" {
			prefix, p.Nice = []string{tp, "-b"}, Applied
		} else {
			p.Reason = "taskpolicy not found"
		}
		if sh.CPUCapped || sh.MemCapped {
			p.CPU, p.Memory = NoInfo, NoInfo
			p.Reason = join(p.Reason, "cgroup limits are Linux only; the reservation is held by the slot count")
		}
	case "linux":
		if nice := pl.first("/usr/bin/nice", "/bin/nice"); nice != "" {
			prefix, p.Nice = []string{nice, "-n", "10"}, Applied
		} else {
			p.Reason = "nice not found"
		}
		if sh.CPUCapped || sh.MemCapped {
			prefix, p = i.joinGroup(pl, s, sh, prefix, p)
		}
	default:
		p.Reason = "no background priority on " + pl.GOOS
	}
	s.Command = append(append([]string{}, prefix...), s.Command...)
	return p
}

// joinGroup adds the cgroup stub in front of prefix when at least one limit could be set.
func (i *Isolator) joinGroup(pl Platform, s *exec.JobSpec, sh reservation.Share, prefix []string, p Priority) ([]string, Priority) {
	sh1 := pl.first("/bin/sh", "/usr/bin/sh")
	pl.Sweep(sweepAge)
	var dir string
	parent, cpuErr, memErr, err := pl.jobParent(sh)
	if err == nil && sh1 != "" {
		dir, err = leaf(parent, s.AttemptID)
	}
	if err != nil || sh1 == "" {
		p.CPU, p.Memory = NoInfo, NoInfo
		p.Reason = join(p.Reason, "cgroup limits not applied: "+reasonOf(err, sh1))
		return prefix, p
	}
	if sh.CPUCapped {
		p.CPU = limitState(cpuErr, &p)
	}
	if sh.MemCapped {
		p.Memory = limitState(memErr, &p)
	}
	if p.CPU != Applied && p.Memory != Applied { // nothing set: do not join a group that limits nothing
		return prefix, p
	}
	stub := []string{sh1, "-c", `echo $$ > "$0/cgroup.procs" && exec "$@"`, filepath.Clean(dir)}
	return append(stub, prefix...), p
}

// limitState is Applied, or NoInfo with the write error added to the report.
func limitState(err error, p *Priority) string {
	if err == nil {
		return Applied
	}
	p.Reason = join(p.Reason, err.Error())
	return NoInfo
}

func reasonOf(err error, sh string) string {
	if err != nil {
		return err.Error()
	}
	if sh == "" {
		return "sh not found"
	}
	return ""
}

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}
