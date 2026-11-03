package run

// Purpose: the lifecycle of a run's dispatcher: the process map of dispatchers,
// their release when the run is recorded, the wake of queued jobs, and the
// incarnation fenced leases carry. Split from dispatch_hook.go by responsibility.
// Inputs:  a run id; the store for the incarnation.
// Outputs: dispatchers closed and released, queued jobs woken.
// Constraints: imports no internal/dispatch (layers.yaml: only dispatch_hook.go may);
// it reaches the dispatcher through the runDispatch type declared there.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

var (
	runDispatchers sync.Map   // run id -> *runDispatch
	runDispatchMu  sync.Mutex // one construction at a time: OnStart hooks run once per dispatcher
)

// dispatchCleanup closes a run's dispatcher when the run is recorded.
type dispatchCleanup struct{}

func (dispatchCleanup) AfterRun(_ context.Context, r RunInfo) error {
	dispatchRelease(r.RunID)
	return nil
}

// dispatchRelease closes the dispatcher of a run. Coordinator.Run defers it, so a run that
// ends on an error path before AfterRun does not leave its dispatcher in the process map (D-1044).
func dispatchRelease(runID string) {
	ctx, cancel := background(context.Background())
	defer cancel()
	_ = (remoteCacheFinalize{}).AfterRun(ctx, RunInfo{RunID: runID})
	if v, ok := runDispatchers.LoadAndDelete(runID); ok {
		rd := v.(*runDispatch)
		rd.d.Close()
		rd.sink.release()
		if rd.econRestore != nil {
			rd.econRestore()
		}
	}
}

// dispatchWake lets the run's queued jobs tick now: an attempt ended and its
// host reservation is free.
func dispatchWake(runID string) {
	if v, ok := runDispatchers.Load(runID); ok {
		v.(*runDispatch).d.Wake()
	}
}

// highestIncarnation is the incarnation fenced leases carry: the highest of the coordinator table, so a
// leased epoch after a `serve restore` exceeds every epoch granted before the backup (P7-NODE-20).
func highestIncarnation(st *store.Store) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, _ := st.HighestIncarnation(ctx) // a failed read is 1, the old behaviour
	return n
}
