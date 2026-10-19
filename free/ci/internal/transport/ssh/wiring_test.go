package ssh

// Purpose: regression tests for debt D-1054 (ssh.Default was never configured by a
// composition root): the lease TTL drives agent_ttl, the repository is resolved per
// lease, and Configure refuses a wiring that would fail at the first delivery.
// Inputs: none. Outputs: test results.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

func noRenew(context.Context, string, uint64) error { return nil }

// TestAgentTTLIsDerivedFromTheLeaseTTL proves agent_ttl = coordinator_ttl - 2 heartbeats
// (Epic NODE D6) when the composition root states the lease TTL, and that the coordinator's
// silence bound is then exactly the lease TTL.
func TestAgentTTLIsDerivedFromTheLeaseTTL(t *testing.T) {
	cases := []struct {
		name               string
		d                  *Driver
		wantAgent, wantCoo time.Duration
	}{
		{"default", &Driver{}, 25 * time.Second, 45 * time.Second},
		{"lease 45s, heartbeat 10s", &Driver{LeaseTTL: 45 * time.Second}, 25 * time.Second, 45 * time.Second},
		{"lease 90s, heartbeat 10s", &Driver{LeaseTTL: 90 * time.Second}, 70 * time.Second, 90 * time.Second},
		{"lease 12s, heartbeat 2s", &Driver{LeaseTTL: 12 * time.Second, Heartbeat: 2 * time.Second}, 8 * time.Second, 12 * time.Second},
		{"explicit AgentTTL wins", &Driver{LeaseTTL: 90 * time.Second, AgentTTL: 30 * time.Second}, 30 * time.Second, 50 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.agentTTL(); got != c.wantAgent {
				t.Errorf("agentTTL = %v, want %v", got, c.wantAgent)
			}
			if got := c.d.coordinatorTTL(); got != c.wantCoo {
				t.Errorf("coordinatorTTL = %v, want %v", got, c.wantCoo)
			}
		})
	}
}

// TestBundleUsesThePerLeaseRepository proves a coordinator serving several projects bundles the
// repository the lease belongs to, not the working directory.
func TestBundleUsesThePerLeaseRepository(t *testing.T) {
	repoA, revA := makeRepo(t)
	repoB, revB := makeRepo(t)
	seen := map[string]string{}
	d := &Driver{NoRenew: true, RepoFor: func(_ context.Context, l protocol.Lease) (string, error) {
		seen[l.LeaseID] = l.Checkout.Revision
		if l.LeaseID == "b" {
			return repoB, nil
		}
		return repoA, nil
	}}
	for _, c := range []struct{ id, rev string }{{"a", revA}, {"b", revB}} {
		l := testLease()
		l.LeaseID, l.Checkout.Revision = c.id, c.rev
		path, cleanup, err := d.makeBundle(context.Background(), &l)
		if err != nil {
			t.Fatalf("lease %s: %v", c.id, err)
		}
		if l.Checkout.BundleDigest == "" {
			t.Errorf("lease %s: no bundle digest recorded", c.id)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("lease %s: %v", c.id, err)
		}
		cleanup()
	}
	if len(seen) != 2 {
		t.Fatalf("RepoFor saw %v, want both leases", seen)
	}
}

// TestBundleRefusesWhenTheRepositoryCannotBeResolved proves a lease whose project is unknown
// fails with the checkout code and never falls back to the working directory.
func TestBundleRefusesWhenTheRepositoryCannotBeResolved(t *testing.T) {
	d := &Driver{NoRenew: true, RepoDir: t.TempDir(), RepoFor: func(context.Context, protocol.Lease) (string, error) {
		return "", errors.New("no project for lease")
	}}
	l := testLease()
	l.Checkout.Revision = "HEAD"
	_, _, err := d.makeBundle(context.Background(), &l)
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonCheckout {
		t.Fatalf("err = %v, want a %s transport error", err, ReasonCheckout)
	}
}

// TestConfigureSetsTheDriverAndRefusesAnUnrunnableWiring proves the composition root gets a
// loud error at wiring time instead of transport.ssh_not_configured at the first delivery.
func TestConfigureSetsTheDriverAndRefusesAnUnrunnableWiring(t *testing.T) {
	var d Driver
	if err := d.Configure(Wiring{LeaseTTL: 45 * time.Second}); err == nil {
		t.Error("a wiring without Renew was accepted")
	}
	if err := d.Configure(Wiring{Renew: noRenew, LeaseTTL: 20 * time.Second}); err == nil {
		t.Error("a lease TTL below three heartbeats was accepted")
	}
	if err := d.Configure(Wiring{Renew: noRenew, Heartbeat: 10 * time.Second, LeaseTTL: 29 * time.Second}); err == nil {
		t.Error("a lease TTL below three explicit heartbeats was accepted")
	}
	w := Wiring{Renew: noRenew, Personal: true, MinAgentVersion: "1.5.0", LeaseTTL: 60 * time.Second,
		RepoFor: func(context.Context, protocol.Lease) (string, error) { return "/r", nil }}
	if err := d.Configure(w); err != nil {
		t.Fatal(err)
	}
	if d.Renew == nil || !d.Personal || d.MinAgentVersion != "1.5.0" || d.LeaseTTL != 60*time.Second || d.RepoFor == nil {
		t.Error("Configure left the driver unset")
	}
	if got := d.agentTTL(); got != 40*time.Second {
		t.Errorf("agentTTL = %v, want 40s", got)
	}
}

// TestUnconfiguredDefaultStillRefusesToRun keeps the guard: a driver nobody configured never
// starts a job that no heartbeat would renew.
func TestUnconfiguredDefaultStillRefusesToRun(t *testing.T) {
	node, _ := testNode(t)
	err := (&Driver{}).Run(context.Background(), node, testLease(), &recSink{})
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonNotConfigured {
		t.Fatalf("err = %v, want %s", err, ReasonNotConfigured)
	}
}
