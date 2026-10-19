package world

// Purpose: the one way `ci plan` and a run describe the local runner and build
// the placement world around it (P7-AUTO-28, D-0989, D-1042): same inputs, same
// world, same digest.
// Inputs:  Deps (store, policy, jobs, clock), the facts observed about this
// machine (LocalInput) and the pipeline.
// Outputs: the world and its digest, exactly as Build returns them.
// Constraints: the local runner is declared once, by Build (the PERSONAL own
// node); a registered node that already uses the reserved id "local" is refused
// here, never silently shadowed.

import (
	"context"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

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
	if n != 1 { // a registered node called "local" would inherit the personal-node declaration
		return sched.World{}, "", failed("the node registry holds a node with the reserved id \"local\"", nil)
	}
	return w, digest, nil
}
