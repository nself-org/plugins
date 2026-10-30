package pools

// Purpose: the autoworld exclusion provider (P7-AUTO-27): every runner the pool terms drop carries
//          pool.excluded with its detail, so `ci plan` and `ci explain` show it like any other reason.
// Inputs:  Config: the assigned-label reader, whether the store is a TEAM coordinator's, the dev-run test
//          and the optional enrolment source; the world's resolved policy (w.Policy.Trust) gives the terms.
// Outputs: autoworld.ExclusionFn; Register adds it under ProviderName; RunnersFromWorld.
// Constraints: membership comes from the label reader only (the world's runner labels mix in the node's
//          self-declared labels, so they are never read). With every term unset the provider reads
//          nothing and returns nothing, so placement is byte-identical to a build without pools. The
//          authorized_projects term is left to placement's own reason.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/labels"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ProviderName is the name the provider registers under.
const ProviderName = "pools"

// localRunnerID is the id world.BuildForLocal gives this machine's runner.
const localRunnerID = "local"

// LabelReader reads one runner's coordinator-assigned labels; *store.Store and labels.LazyReader satisfy it.
type LabelReader interface {
	NodeLabels(ctx context.Context, nodeID string) ([]store.NodeLabel, error)
}

// Config wires the provider.
type Config struct {
	Labels LabelReader
	// Team reports whether the store is a TEAM coordinator's: its nodes are then reached through it.
	Team func() bool
	// Dev reports whether the run being planned is a dev run; nil means never.
	Dev func() bool
	// Enrolled returns the pools the person is enrolled in; nil means every pool they see.
	Enrolled func(ctx context.Context) (List, error)
}

// ExclusionProvider returns the provider for c.
func ExclusionProvider(c Config) autoworld.ExclusionFn {
	return func(ctx context.Context, w sched.World) (map[string][]model.PlacementReasonDetail, error) {
		allow, devPools, _ := FromPolicy(w.Policy.Trust)
		in := Input{Allow: allow, DevPools: devPools, Dev: c.Dev != nil && c.Dev()}
		if c.Enrolled != nil {
			var err error
			if in.Enrolled, err = c.Enrolled(ctx); err != nil {
				return nil, err
			}
		}
		if !in.Allow.Set && !in.Enrolled.Set && !(in.Dev && in.DevPools.Set) {
			return nil, nil
		}
		var err error
		if in.Runners, err = RunnersFromWorld(ctx, w, c); err != nil {
			return nil, err
		}
		got := Resolve(in).Excluded
		if len(got) == 0 {
			return nil, nil
		}
		return got, nil
	}
}

// Register adds the provider to autoworld. Call it once from the composition root.
func Register(c Config) error {
	return autoworld.RegisterExclusion(ProviderName, ExclusionProvider(c))
}

// RunnersFromWorld turns the world's runners into resolver runners with their assigned labels.
func RunnersFromWorld(ctx context.Context, w sched.World, c Config) ([]Runner, error) {
	team := c.Team != nil && c.Team()
	out := make([]Runner, 0, len(w.Runners)+len(w.Hosted))
	for _, r := range w.Runners {
		src := SourceRegistry
		switch {
		case r.ID == localRunnerID:
			src = SourceLocal
		case team:
			src = SourceCoordinator
		}
		got := Runner{ID: r.ID, Location: r.Location, Source: src, Projects: r.Capability.Lifecycle.AuthorizedProjects}
		if c.Labels != nil {
			set, err := c.Labels.NodeLabels(ctx, r.ID)
			if err != nil {
				return nil, err
			}
			got.Assigned = labels.Strings(set)
		}
		out = append(out, got)
	}
	for _, r := range w.Hosted {
		loc := r.Location
		if loc == "" {
			loc = "hosted"
		}
		out = append(out, Runner{ID: r.ID, Location: loc, Source: SourceProvider})
	}
	return out, nil
}
