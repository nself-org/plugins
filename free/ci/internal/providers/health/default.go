package health

// Purpose: the process-wide tracker and its registration (P7-HOST-11): importing this package makes
//          providers.HealthOf answer from observed outcomes and makes every REST funnel client
//          report its outcomes here (rest.SetDefaultObserver), with no change to a client builder.
// Inputs:  the coordinator's *store.Store through UseStore.
// Outputs: Default(); providers.RegisterHealth and rest.SetDefaultObserver are called from init().
// Constraints: dormant unless compat.V15() (the observer and the health function are no-ops under
//          v1.4); the notify webhook client is not a provider API and is not tracked; until the
//          coordinator calls UseStore the breaker works in memory only.

import (
	"context"
	"sync"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

var (
	defaultOnce    sync.Once
	defaultTracker *Tracker
)

// Default is the process-wide tracker.
func Default() *Tracker {
	defaultOnce.Do(func() {
		defaultTracker = New(Options{Drain: report.Drain, Ignore: []string{"notify"}})
	})
	return defaultTracker
}

// UseStore makes the process-wide tracker persist its state in the coordinator's store.
func UseStore(s Store) { Default().SetStore(s) }

func init() { Install(Default()) }

// Install makes t answer providers.HealthOf and receive the outcome of every REST funnel client.
// init() installs the process-wide tracker; a test installs its own and puts Default() back.
func Install(t *Tracker) {
	providers.RegisterHealth(func(ctx context.Context, id string) providers.Health {
		if !compat.V15() {
			return providers.Health{State: "up"}
		}
		h := t.HealthOf(ctx, id)
		if h.State != "up" {
			return h
		}
		if p, ok := providers.Lookup(id); ok { // observed outcomes say up: a provider's own probe may still say more
			if probe, ok := p.(providers.HealthProbe); ok {
				return probe.Health(ctx)
			}
		}
		return h
	})
	rest.SetDefaultObserver(func(o rest.Outcome) {
		if compat.V15() {
			t.Observe(o)
		}
	})
}
