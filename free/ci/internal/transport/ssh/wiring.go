package ssh

// Purpose: the one entry point a composition root uses to wire the shared ssh driver
// (debt D-1054): lease renewal, secret source, coordinator mode, minimum agent version, the
// lease TTL that drives agent_ttl, and the per-lease repository.
// Inputs:  a Wiring. Outputs: the driver's fields set, or an error that names what is missing.
// Constraints: call before the first delivery; the fields are read without a lock while a
// lease runs, so Configure is not safe against a concurrent Run.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Wiring is what the composition root knows and the driver cannot.
type Wiring struct {
	// Renew extends the lease on each agent heartbeat (internal/lease Engine.Renew); required.
	Renew func(ctx context.Context, leaseID string, epoch uint64) error
	// Secrets resolves operator secret values; used only when Personal is true.
	Secrets SecretSource
	// Personal marks a PERSONAL coordinator.
	Personal bool
	// MinAgentVersion is the oldest agent the coordinator accepts (empty accepts any).
	MinAgentVersion string
	// LeaseTTL is the lease engine's TTL; agent_ttl derives from it. Required, and at least
	// three heartbeats so agent_ttl stays at or above one heartbeat.
	LeaseTTL time.Duration
	// Heartbeat overrides the 10 s default; zero keeps it.
	Heartbeat time.Duration
	// RepoFor maps a lease to its repository (a TEAM coordinator serves several projects).
	RepoFor func(ctx context.Context, l protocol.Lease) (string, error)
	// IdentityFile is an optional private key offered to every node.
	IdentityFile string
}

// Configure applies w, or returns an error and leaves the driver unchanged.
func (d *Driver) Configure(w Wiring) error {
	if w.Renew == nil {
		return errors.New("ssh transport wiring: Renew is required (lease renewal on agent heartbeats)")
	}
	hb := max(orDefault(w.Heartbeat, 10*time.Second), time.Second)
	if w.LeaseTTL < 3*hb {
		return fmt.Errorf("ssh transport wiring: LeaseTTL %v must be at least three heartbeats (%v)", w.LeaseTTL, 3*hb)
	}
	d.Renew, d.NoRenew = w.Renew, false
	d.Secrets, d.Personal = w.Secrets, w.Personal
	d.MinAgentVersion, d.LeaseTTL, d.Heartbeat = w.MinAgentVersion, w.LeaseTTL, w.Heartbeat
	d.RepoFor, d.IdentityFile = w.RepoFor, w.IdentityFile
	return nil
}
