package dispatch

// Purpose: small pure helpers of the tick: runner lookup, capacity, and the
// human reason lines of a job that waits or cannot run.

import (
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

type placement = sched.Placement

func (d *Dispatcher) incarnation() int64 {
	if d.cfg.Incarnation != nil {
		return d.cfg.Incarnation()
	}
	return 1
}

func runnerOf(w *sched.World, id string) (sched.Runner, bool) {
	for _, r := range w.Runners {
		if r.ID == id {
			return r, true
		}
	}
	for _, r := range w.Hosted {
		if r.ID == id {
			return r, true
		}
	}
	return sched.Runner{}, false
}

// slotsOf returns the runner's declared slots and whether the figure is known.
func slotsOf(r sched.Runner) (int, bool) {
	if s := r.Capability.Availability.Slots.Value; s != nil {
		return *s, true
	}
	return 0, false
}

// capacity is the host capacity a placement reserves against.
func capacity(r sched.Runner, j Job, local bool) (cpu, mem int) {
	if v := r.Capability.Resources.CPU.Value; v != nil {
		cpu = int(*v)
	}
	if v := r.Capability.Resources.MemMB.Value; v != nil {
		mem = int(*v)
	}
	if local {
		if j.LocalCPU > 0 {
			cpu = j.LocalCPU
		}
		if j.LocalMemMB > 0 {
			mem = j.LocalMemMB
		}
	}
	return cpu, mem
}

// waitReason is the reason `ci explain` shows for a job that waits: the first
// specific reason of the first runner that has one, else the job-level reason.
func waitReason(x sched.Explanation) model.PlacementReason {
	for _, a := range x.Alternatives {
		for _, r := range a.Reasons {
			if r.Code != string(model.CapacityBusy) {
				return model.PlacementReason(r.Code)
			}
		}
	}
	if len(x.Reasons) > 0 {
		return model.PlacementReason(x.Reasons[0].Code)
	}
	for _, a := range x.Alternatives {
		if len(a.Reasons) > 0 {
			return model.PlacementReason(a.Reasons[0].Code)
		}
	}
	return ""
}

// unplaceableLine is the one human reason line of an E618 (X17).
func unplaceableLine(j Job, x sched.Explanation) string {
	var parts []string
	for i, a := range x.Alternatives {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more runners", len(x.Alternatives)-3))
			break
		}
		codes := make([]string, 0, len(a.Reasons))
		for _, r := range a.Reasons {
			codes = append(codes, r.Code)
		}
		parts = append(parts, fmt.Sprintf("%s: %s", a.RunnerID, strings.Join(codes, ",")))
	}
	if len(parts) == 0 {
		for _, r := range x.Reasons {
			parts = append(parts, r.Code)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "no runner is registered")
	}
	return fmt.Sprintf("job %s cannot run on any runner (%s)", j.Sched.Name, strings.Join(parts, "; "))
}
