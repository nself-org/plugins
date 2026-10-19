package schedtel

// Purpose: wires the package into the dispatcher, the run loop and the world.
// Inputs:  none (init).
// Outputs: the dispatch sink factory, the run finalizer and the `schedtel`
// contributor, registered once per process.
// Constraints: registration is unconditional but inert outside v1.5 mode, where
// no dispatcher exists, no decision is recorded and no store is bound.

import (
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func init() {
	factory := func(st *store.Store, _ string) dispatch.DecisionSink {
		if st == nil {
			return nil
		}
		c := clock()
		return NewSink(st, func() int64 { return c().UnixMilli() })
	}
	if err := dispatch.RegisterSinkFactory(factory); err != nil {
		panic(err)
	}
	if err := registerContributor(); err != nil {
		panic(err)
	}
	cirun.RegisterFinalizer(finalizer{})
}
