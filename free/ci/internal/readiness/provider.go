package readiness

// Purpose: the autoworld exclusion provider. A local runner that is not ready carries
// the reason node.not_ready, so `ci plan` shows it and sched.Place skips it. The reason
// is not Static: it clears the next time the host answers ready, and the bounded wait of
// the auto preset (AUTO D11) may wait for it.
// Inputs: a Source (the Evaluator) and an optional foreground test.
// Outputs: autoworld.ExclusionFn; Register adds it under ProviderName.
// Constraints: only the runner carrying world.LocalLabel is judged (remote nodes keep
// NODE-13's agent-side facts). Foreground floor (X17): the operator's own foreground run
// keeps its one local slot whatever the answer, so a foreground run adds no exclusion.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// ProviderName is the name the provider registers under (providers run in name order).
const ProviderName = "readiness"

// Source is what the provider needs from an Evaluator.
type Source interface {
	Evaluate(ctx context.Context) (Report, error)
}

// ExclusionProvider returns the provider for src. foreground may be nil (never foreground).
func ExclusionProvider(src Source, foreground func() bool) autoworld.ExclusionFn {
	return func(ctx context.Context, w sched.World) (map[string][]model.PlacementReasonDetail, error) {
		if foreground != nil && foreground() {
			return nil, nil
		}
		var local []string
		for _, r := range w.Runners {
			for _, l := range r.Capability.Identity.Labels {
				if l == world.LocalLabel {
					local = append(local, r.ID)
					break
				}
			}
		}
		if len(local) == 0 {
			return nil, nil
		}
		rep, err := src.Evaluate(ctx)
		if err != nil {
			return nil, err
		}
		if rep.Ready {
			return nil, nil
		}
		detail := model.PlacementReasonDetail{Code: string(model.NodeNotReady), Detail: ClipLine(strings.Join(rep.Why, "; "))}
		out := make(map[string][]model.PlacementReasonDetail, len(local))
		for _, id := range local {
			out[id] = []model.PlacementReasonDetail{detail}
		}
		return out, nil
	}
}

// Register adds the provider to autoworld. Call it once at startup from the composition
// root that owns the Evaluator.
func Register(src Source, foreground func() bool) error {
	return autoworld.RegisterExclusion(ProviderName, ExclusionProvider(src, foreground))
}
