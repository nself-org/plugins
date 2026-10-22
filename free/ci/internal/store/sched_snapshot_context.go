package store

// Purpose: pin durable placement facts across all pipeline world builds in a tick.
// Inputs: a store and context. Outputs: a context carrying one read snapshot.
// Constraints: admission uses its own transaction; callers must scope this context
// to world building. Each reader receives independent mutable maps and slices.
import (
	"context"
	"maps"
	"slices"
)

type schedSnapshotKey struct{}
type schedSnapshotContext struct {
	store *Store
	world SchedWorld
}

// WithWorldSnapshot pins WorldSnapshot reads of this store to one instant.
func (s *Store) WithWorldSnapshot(ctx context.Context) (context.Context, error) {
	snapshot, err := s.WorldSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, schedSnapshotKey{}, schedSnapshotContext{s, snapshot}), nil
}

func (s *Store) pinnedWorld(ctx context.Context) (SchedWorld, bool) {
	pinned, ok := ctx.Value(schedSnapshotKey{}).(schedSnapshotContext)
	if !ok || pinned.store != s {
		return SchedWorld{}, false
	}
	w := pinned.world
	w.Nodes = slices.Clone(w.Nodes)
	for i := range w.Nodes {
		w.Nodes[i].Capability = slices.Clone(w.Nodes[i].Capability)
	}
	w.Active = slices.Clone(w.Active)
	w.Queued = maps.Clone(w.Queued)
	w.Previous = maps.Clone(w.Previous)
	w.PreviousByJob = maps.Clone(w.PreviousByJob)
	w.Reservations = maps.Clone(w.Reservations)
	w.Projects = maps.Clone(w.Projects)
	return w, true
}
