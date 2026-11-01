package evidence

// Purpose: answer a finished rerun from the store, the way the next verify will.
// Inputs: the evidence env, a revision and the run's in-memory document.
// Outputs: the newest stored document of that run, or the in-memory one.
// Constraints: only the run's own documents count; a store error keeps the in-memory answer.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// recordedAfter returns the newest document the store holds for the run that
// just finished, or ev when it holds none. A hook can append a later document
// to a run (the record-pool gate stores one carrying E809 beside the run's
// own); the next `verify` answers from the store, newest first, so this one
// must answer the same document or a refusal would show on every call but the
// first. Only the run's own documents count, never another run of the revision.
func (env Env) recordedAfter(ctx context.Context, rev string, ev model.Evidence) model.Evidence {
	if env.Store == nil || ev.RunID == "" {
		return ev
	}
	docs, err := ForRevision(ctx, env.Store, rev)
	if err != nil {
		return ev
	}
	for _, d := range docs { // newest first; same-instant rows are ordered by id (sortStored)
		if d.Evidence.RunID == ev.RunID {
			return d.Evidence
		}
	}
	return ev
}
