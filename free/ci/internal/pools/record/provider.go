package record

// Purpose: exclude non-record runners before checkout or admission.
// Inputs: trusted label reader, run kind and resolved placement world.
// Outputs: autoworld exclusions. Constraints: unreadable labels exclude all.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/autoworld"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// ExclusionProvider checks commit-CI only; dev runs use pools.dev instead.
func ExclusionProvider(c pools.Config, isRecord func(context.Context) bool) autoworld.ExclusionFn {
	return func(ctx context.Context, w sched.World) (map[string][]model.PlacementReasonDetail, error) {
		if isRecord == nil || !isRecord(ctx) {
			return nil, nil
		}
		runners, err := pools.RunnersFromWorld(ctx, w, c)
		if err != nil || c.Labels == nil {
			out := map[string][]model.PlacementReasonDetail{}
			for _, rs := range [][]sched.Runner{w.Runners, w.Hosted} {
				for _, r := range rs {
					out[r.ID] = []model.PlacementReasonDetail{{Code: string(model.PoolExcluded), Detail: Reason}}
				}
			}
			return out, nil
		}
		return Excluded(FromPolicy(w.Policy.Trust), runners), nil
	}
}
