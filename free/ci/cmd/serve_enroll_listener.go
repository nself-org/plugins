package main

// Purpose: the burst enrollment listener of `ci serve` (P7-NODE-20, Epic D5, SCHED review F4):
//          coordinator.RegisterListener("enroll", --enroll-addr, ":3866", ...) whose start runs
//          pairing.ServeWhileOffers. That listener binds nothing until an intent-bound offer is live and
//          closes within 1 s of the last one ending.
// Inputs:  the operator-scope policy through internal/policy's loader (the one behind `ci policy`): the
//          burst.providers.<id>.enabled keys P7-SCHED registers; the pairing service the composition root
//          registered with pairing.Use; the address from --enroll-addr.
// Outputs: a listener that is bound only while the gate is open AND an intent-bound offer is live.
// Constraints: the gate is read when the listener starts, never from the revision under test: the resolver
//          is called with no revision directory, no repository and no per-run values, so a
//          .nself/ci.yaml value cannot enable it (X1; spend keys are also refused from that scope);
//          the gate fails closed (an unreadable policy leaves the listener unbound); the goroutine that
//          runs ServeWhileOffers is awaited by stop. The drain package is imported for its
//          --drain-deadline flag registration.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	_ "github.com/nself-org/plugins/free/ci/internal/coordinator/drain" // registers --drain-deadline through coordinator.RegisterFlag
	"github.com/nself-org/plugins/free/ci/internal/pairing"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const enrollDefaultAddr = ":3866"

func init() {
	if err := coordinator.RegisterListener("enroll", "enroll-addr", enrollDefaultAddr, startEnrollListener); err != nil {
		panic(err)
	}
}

// burstGate is what the operator policy says about burst enrollment.
type burstGate struct {
	Enabled   bool   // at least one ephemeral burst provider is enabled
	PublicURL string // burst.cloudvm.enroll_url, "" when unset
}

var (
	// enrollGate reads the gate; tests replace it.
	enrollGate = readBurstGate
	// enrollPolicyStore opens the store the operator policy rows live in. `ci serve` should hand over its own
	// store with useEnrollStore; the fallback opens the default CI home (NSELF_CI_HOME).
	enrollPolicyStore = openDefaultPolicyStore
	enrollLogf        = func(format string, a ...any) { fmt.Fprintf(os.Stderr, "nself-ci-serve: "+format+"\n", a...) }
)

// useEnrollStore makes the enroll gate read operator policy from st (the coordinator's own store).
func useEnrollStore(st *store.Store) {
	enrollPolicyStore = func() (policy.Reader, func(), error) { return st, func() {}, nil }
}

func openDefaultPolicyStore() (policy.Reader, func(), error) {
	st, err := cirun.OpenStore(filepath.Join(records.StoreDir(""), "state.db"))
	if err != nil {
		return nil, nil, err
	}
	return st, func() { _ = st.Close() }, nil
}

// noGit answers every git call with "no remotes", so the resolver sees no repository and adds neither a
// protected-branch nor a revision scope.
func noGit(context.Context, ...string) (string, int, error) { return "", 0, nil }

// readBurstGate resolves the operator-scope policy (user, team rows; defaults) and reports whether any
// burst provider is enabled. Nothing is read from a worktree: Revision is empty.
func readBurstGate(ctx context.Context) (burstGate, error) {
	rd, closeFn, err := enrollPolicyStore()
	if err != nil {
		return burstGate{}, err
	}
	defer closeFn()
	res := policy.Resolver{Store: rd, Run: noGit}
	got, err := res.Resolve(ctx, policy.Repo{Root: ".", Visibility: "unknown", Class: "owner"}, policy.Revision{})
	if err != nil {
		return burstGate{}, err
	}
	sp, err := schedpolicy.Spend(&got)
	if err != nil {
		return burstGate{}, err
	}
	g := burstGate{PublicURL: sp.Burst.CloudVM.EnrollURL}
	for _, on := range sp.Burst.Enabled {
		g.Enabled = g.Enabled || on
	}
	return g, nil
}

// startEnrollListener is the ListenerStart of the enroll listener. A closed or unreadable gate returns a
// no-op stop without binding anything; an open gate runs pairing.ServeWhileOffers until stop.
func startEnrollListener(ctx context.Context, addr string, _ *coordinator.Coordinator) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	gctx, cancelGate := context.WithTimeout(ctx, 10*time.Second)
	gate, err := enrollGate(gctx)
	cancelGate()
	if err != nil {
		enrollLogf("enroll listener disabled: the burst policy could not be read: %v", err)
		return noop, nil
	}
	if !gate.Enabled {
		return noop, nil
	}
	public := gate.PublicURL
	if public == "" {
		if public, err = defaultPublicURL(addr, interfaceIPs()); err != nil {
			enrollLogf("enroll listener disabled: %v", err)
			return noop, nil
		}
	}
	lctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if serr := pairing.ServeWhileOffers(lctx, addr, public); serr != nil {
			if errors.Is(serr, pairing.ErrNoService) {
				enrollLogf("enroll listener idle: no pairing service is registered (pairing.Use)")
				return
			}
			enrollLogf("enroll listener stopped: %v", serr)
		}
	}()
	return func(sctx context.Context) error {
		cancel()
		select {
		case <-done:
			return nil
		case <-sctx.Done():
			return sctx.Err()
		}
	}, nil
}
