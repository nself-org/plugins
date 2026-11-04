package main

// Purpose: the hosted lease half of the V15 coordinator start (D-1297): it registers a lease profile for every
//          registered hosted provider on the coordinator's lease engine and runs the engine's hosted poll loop,
//          so a lease on a hosted runner is renewed, expired or finished from the provider's view of the run.
// Inputs:  the open coordinator store and the lease engine serveWire built (the one its sweeper uses).
// Outputs: serveHostedLeases returns the func that stops the poll loop and waits for it.
// Constraints: called once per engine at coordinator start (serveWire is V15 only); the profiles read the
//          handles from the same store-backed book the hosted driver writes. A provider whose profile cannot
//          be registered is logged and left unpolled, the others still run; a failed poll is logged and
//          retried on the next tick. Placement is not touched here: the poll loop only watches leases the
//          dispatcher already admitted under the economics binding.

import (
	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// hostedPollEvery is the tick of the hosted poll loop (each lease is still polled at its profile's Poll).
var hostedPollEvery = lease.DefaultHostedTick

// hostedPollObserve, when set, sees every hosted poll the coordinator runs (tests).
var hostedPollObserve func(store.HostedLease, lease.LiveState, error)

// serveHostedLeases registers the hosted lease profiles on eng and starts its hosted poll loop.
func serveHostedLeases(st *store.Store, eng *lease.Engine) (stop func()) {
	if err := runtime.RegisterProfiles(eng, runtime.StoreBook(st)); err != nil {
		agentLogf("hosted lease profiles: %v", err)
	}
	return eng.StartHostedPoller(lease.HostedPollConfig{Every: hostedPollEvery, Logf: agentLogf, Observe: hostedPollObserve})
}
