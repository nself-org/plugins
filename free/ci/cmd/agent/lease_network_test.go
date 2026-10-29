package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// leaseScopeResult is what the executor saw for one (class, scope) lease: the scope it was handed, or that it
// was never called because the agent refused the lease first.
type leaseScopeResult struct {
	ran     bool
	network model.NetworkScope
	state   string
	excerpt string
}

// runLeaseScope runs one container lease of class and declared scope through the agent's lease path, up to the
// isolator (which records the JobSpec and stops), and reports what happened.
func runLeaseScope(t *testing.T, class model.TrustClass, scope model.NetworkScope) leaseScopeResult {
	t.Helper()
	var got ciexec.JobSpec
	cirun.RegisterIsolation(deliveredSpecObserver{&got, errors.New("stop before runtime")})
	cirun.RegisterImage(pinnedImageHook{})
	lease := protocol.Lease{AttemptID: "scope", Trust: class, NetworkScope: scope, Checkout: protocol.Checkout{Revision: rev40},
		Job: model.Job{Isolation: "container", Run: []string{"true"}}}
	res, err := leaseRunner{coordinator: "test"}.Run(context.Background(), session.JobRun{Lease: lease})
	if err != nil {
		t.Fatal(err)
	}
	return leaseScopeResult{ran: got.AttemptID != "", network: got.Network, state: res.Outcome.State, excerpt: string(res.Evidence)}
}

// TestInvariant_I07 points I07 (untrusted never gets LAN) at the agent's lease path (D-1331): for every world of
// the invariant catalogue, a lease that declares lan or privileged for an untrusted job never reaches the
// executor (the attempt fails with E679), any other declared scope reaches it unchanged, and an undeclared scope
// runs as none. The worlds repeat (class, scope) pairs, so each pair runs once.
func TestInvariant_I07(t *testing.T) {
	useState(t)
	old := cirun.Registered
	t.Cleanup(func() { cirun.Registered = old })
	seen := map[string]leaseScopeResult{}
	invariants.Check(t, func(w invariants.World) error {
		key := string(w.Trust) + "/" + string(w.Network)
		r, ok := seen[key]
		if !ok {
			r = runLeaseScope(t, w.Trust, w.Network)
			seen[key] = r
		}
		untrusted := trust.DecodeTrust(string(w.Trust)) == "untrusted"
		lan := w.Network == "lan" || w.Network == "privileged"
		switch {
		case untrusted && lan && r.ran:
			return fmt.Errorf("an untrusted lease declaring %s reached the executor with %q", w.Network, r.network)
		case untrusted && lan && (r.state != "failed" || !strings.Contains(r.excerpt, "E679")):
			return fmt.Errorf("an untrusted lease declaring %s ended %q without E679", w.Network, r.state)
		case untrusted && lan:
			return nil
		case !r.ran:
			return fmt.Errorf("a %s lease declaring %q never reached the executor", w.Trust, w.Network)
		case w.Network == "" && r.network != "none":
			return fmt.Errorf("an undeclared scope ran as %q, want none", r.network)
		case w.Network != "" && r.network != w.Network:
			return fmt.Errorf("a %s lease declaring %s ran as %q", w.Trust, w.Network, r.network)
		}
		return nil
	})
}
