package dispatch

// Test-only exports for the external test package (dispatch_test), which can
// import internal/run (run imports dispatch, so an internal test cannot).

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ResetSteps reopens step registration, keeping the named steps.
func ResetSteps(keep ...string) { resetSteps(keep...) }

func OpenStore(t *testing.T) *store.Store { return openStore(t) }

func QueueAttempt(t testing.TB, st *store.Store, pipe, proj, name string) string {
	return queueAttempt(t, st, pipe, proj, name)
}

func SchedRunner(id, loc string, slots int) sched.Runner { return testRunner(id, loc, slots) }

func QueuedJob(attempt string) Job { return testQueued(attempt) }

func LiveWorld(st *store.Store, runners ...sched.Runner) func(context.Context, sched.Pipeline) (sched.World, string, error) {
	return liveWorld(st, nil, runners...)
}
