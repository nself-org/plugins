package labels

// Purpose: the world label source (P7-SCHED-18): assigned labels join each runner's identity labels,
//          so placement matches them against requires.labels and sched.overrides.labels.
// Inputs:  a Reader over the store's assigned labels.
// Outputs: the placement form of Strings for each runner id the world builder asks about.
// Constraints: world accepts one label source per process; a second RegisterSource fails there (E699).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// Reader reads one node's assigned labels; *store.Store satisfies it.
type Reader interface {
	NodeLabels(ctx context.Context, nodeID string) ([]store.NodeLabel, error)
}

// Source returns the label source function the world builder calls per runner id.
func Source(r Reader) func(context.Context, string) ([]string, error) {
	return func(ctx context.Context, nodeID string) ([]string, error) {
		set, err := r.NodeLabels(ctx, nodeID)
		if err != nil {
			return nil, err
		}
		return Strings(set), nil
	}
}

// RegisterSource installs r as the process-wide world label source.
func RegisterSource(r Reader) error {
	return world.RegisterLabelSource(Source(r))
}
