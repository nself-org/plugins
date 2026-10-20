package main

// Purpose: coordinator mode for `serve` in V15 mode (P7-NODE-20, Epic D21, review F3/F4): the --team flag,
// the pre-bind mode check and the coordinator-facts provider that lets the world and the trust floor
// know a coordinator is serving.
// Inputs:  the parsed serve flag set, the opened coordinator store, ctx of the serve.
// Outputs: serveModeStart returns an end function to call when the serve stops, or a refusal (E664 reason
//          personal_has_principals, or facts_provider_missing).
// Constraints: the provider is registered once at init (world allows one) and reports Serving false until a
//          serve calls serveModeStart; a serve refuses to start while the provider is missing; --team is
//          permanent for the data dir and audited once; nothing here binds a port or reads the network.

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/mode"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// serveFactsRegistered says whether the world has a coordinator-facts provider; tests replace it.
var serveFactsRegistered = world.CoordinatorFactsRegistered

// serveModeActor is the audit actor of a mode switch made by `serve --team`.
const serveModeActor = "operator:serve"

func init() {
	if err := mode.RegisterProvider(); err != nil {
		panic(err)
	}
	coordinator.RegisterFlag(func(fs *flag.FlagSet) {
		fs.Bool("team", false, "Run as a TEAM coordinator (permanent for this data dir, audited). Required once anyone but the operator holds a grant, token or certificate")
	})
}

// serveTeamFlag reads --team from a parsed serve flag set.
func serveTeamFlag(fs *flag.FlagSet) bool {
	f := fs.Lookup("team")
	if f == nil {
		return false
	}
	g, ok := f.Value.(flag.Getter)
	if !ok {
		return false
	}
	v, _ := g.Get().(bool)
	return v
}

// serveModeStart runs the start check before any port is bound: the facts provider must be registered, the
// stored mode resolves (personal by default, team with --team) and a personal data dir with a non-self
// principal is refused. On success the serve is registered as running, so the world sees a coordinator host
// until the returned end function runs.
func serveModeStart(ctx context.Context, fs *flag.FlagSet, st *store.Store, stderr io.Writer) (func(), error) {
	if err := mode.RequireProvider(serveFactsRegistered); err != nil {
		return nil, err
	}
	d, err := mode.Start(ctx, st, serveTeamFlag(fs), serveModeActor)
	if err != nil {
		return nil, err
	}
	if d.Switched {
		_, _ = fmt.Fprintln(stderr, "nself-ci-serve: coordinator mode set to team (audited; it never goes back to personal on this data dir)")
	}
	_, _ = fmt.Fprintf(stderr, "nself-ci-serve: coordinator mode %s\n", d.Mode)
	s := mode.Begin(mode.Options{Store: st})
	return s.End, nil
}
