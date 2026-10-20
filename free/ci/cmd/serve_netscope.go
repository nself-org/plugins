package main

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// serveNetscope reads the coordinator's live cache through its existing store handle.
func serveNetscope(st *store.Store, home string) func() {
	cirun.RegisterNetscopeExclusions(coordinatorExclusions(st, home))
	return func() { cirun.RegisterNetscopeExclusions(nil) }
}

func coordinatorExclusions(st *store.Store, home string) func(context.Context) (netscope.Exclusions, error) {
	guard := deployguard.New(st, home, "coordinator")
	return func(ctx context.Context) (netscope.Exclusions, error) {
		v, err := guard.ExclusionSet(ctx)
		if err != nil {
			return netscope.Exclusions{}, err
		}
		entries, err := deployguard.LoadNeverList(deployguard.ConfigPath(home))
		if err != nil {
			return netscope.Exclusions{}, err
		}
		live := deployguard.NewSet(entries).View()
		return netscope.Exclusions{Hosts: append(v.Hosts, live.Hosts...), Addresses: append(v.Addresses, live.Addresses...), Prefixes: append(v.Prefixes, live.Prefixes...)}, nil
	}
}
