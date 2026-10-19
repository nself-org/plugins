package world

// Purpose: the one way `ci plan` and a run describe the local runner and build
// the placement world around it (P7-AUTO-28, D-0989, D-1042): same inputs, same
// world, same digest.
// Inputs:  Deps (store, policy, jobs, clock), the facts observed about this
// machine (LocalInput) and the pipeline.
// Outputs: the world and its digest, exactly as Build returns them.
// Constraints: the local runner is declared once, by Build (the PERSONAL own
// node); a registered node that uses the id "local" is the local runner, as
// `ci plan` has always treated it, and gets no injected twin.

import (
	"context"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// LocalLabel is the label of the runner that is this machine. A job that must stay here requires it
// (the run's dispatch hook), so no pipeline id or scheduler field is overloaded for the purpose. It is
// reserved: no other runner should carry it.
const LocalLabel = "nself.local"

// LocalInput is what the caller observed about this machine.
type LocalInput struct {
	Project  string
	Platform string // os/arch
	Capacity ciexec.Capacity
	// Docker and Tools are measured facts; DockerUnknown says the caller did not
	// measure docker at all (a run does not probe for it) and wins over Docker.
	Docker        bool
	DockerUnknown bool
	Tools         []string
	Now           time.Time
}

// BuildForLocal builds the world with this machine as the runner "local".
// d.Local and d.LocalCapacity are set here.
func BuildForLocal(ctx context.Context, d Deps, in LocalInput, p sched.Pipeline) (sched.World, string, error) {
	local, err := planview.LocalCapability(planview.LocalFacts{Project: in.Project, Platform: in.Platform, CPUs: in.Capacity.CPUs,
		MemMB: int64(in.Capacity.MemoryMB), Docker: in.Docker, Tools: in.Tools, Now: in.Now})
	if err != nil {
		return sched.World{}, "", err
	}
	if in.DockerUnknown {
		local.Tools.Docker = model.Fact[bool]{Source: "probe", ObservedAt: in.Now, Confidence: "unknown"}
	}
	local.Identity.Labels = append(local.Identity.Labels, LocalLabel)
	capacity := in.Capacity
	d.Local, d.LocalCapacity = &local, &capacity
	w, digest, err := Build(ctx, d, p)
	if err != nil {
		return sched.World{}, "", err
	}
	n := 0
	for _, r := range w.Runners {
		if r.ID == "local" {
			n++
		}
	}
	if n > 1 {
		// a registered node called "local" is the local runner (it was probed by nobody and must not get the
		// owner exception): build again without the injected one
		d.Local = nil
		w, _, err = Build(ctx, d, p)
		if err != nil {
			return sched.World{}, "", err
		}
	}
	// the label means "executes on this machine through the local path": only the injected runner has it,
	// whatever a registered node, a pool or a contributor assigned (a registered node called local is an
	// ssh endpoint and, in that rare setup, local-only jobs have no runner)
	stripped := false
	for i := range w.Runners {
		r := &w.Runners[i]
		if r.ID == "local" && n == 1 { // the injected runner: no registered node has this id
			continue
		}
		var keep []string
		had := false
		for _, l := range r.Capability.Identity.Labels {
			if l == LocalLabel {
				had = true
				continue
			}
			keep = append(keep, l)
		}
		if had {
			r.Capability.Identity.Labels, stripped = keep, true
		}
	}
	if stripped || n > 1 {
		digest, err = Digest(w)
		if err != nil {
			return sched.World{}, "", err
		}
	}
	return w, digest, nil
}
