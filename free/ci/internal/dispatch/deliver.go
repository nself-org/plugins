package dispatch

// Purpose: delivery of an admitted remote or provider lease (Epic D7, HOST delta
// 6). After the admission commits, the lease goes to its node through the
// transport layer: registered preparers first, then the driver registered for
// the node's transport kind (transport.Deliver). A local attempt is not
// delivered: the waiting caller runs it on the P7-CI-37 executor path.
// Inputs:  an Admitted lease, its runner and the job definition.
// Outputs: the carrier run in the background; a failed delivery marks the
// attempt lost so the lease engine's retry rules apply.
// Constraints: dispatch cannot import internal/protocol (layers.yaml), so the
// protocol.Lease is built by the Transport adapter run/dispatch_hook.go injects;
// an unknown transport kind is checked before admission (node.protocol), never
// after it.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Delivery is what the carrier needs to start one leased attempt.
type Delivery struct {
	Node      model.Capability
	Job       model.Job
	AttemptID string
	LeaseID   string
	// Token is the lease secret; Epoch is the fencing epoch of the lease.
	Token string
	Epoch int64
	// Revision is the commit to check out; ExpiresAt is when the lease lapses
	// without a heartbeat.
	Revision  string
	ExpiresAt time.Time
	// Hosted marks a provider lease (the runner is not an enrolled node).
	Hosted bool
}

// Transport delivers leases. The run package adapts internal/transport to it.
type Transport interface {
	// Known reports whether a driver is registered for the transport kind.
	Known(kind string) bool
	// Deliver runs the preparers, then the driver; it returns when the carrier ends.
	Deliver(ctx context.Context, d Delivery) error
}

func (d *Dispatcher) knownTransport(r sched.Runner) bool {
	return d.cfg.Transport != nil && d.cfg.Transport.Known(r.Capability.Identity.Transport)
}

// deliver starts the carrier for a committed remote lease.
func (d *Dispatcher) deliver(q Queued, r sched.Runner, a Admitted) {
	delivery := Delivery{Node: r.Capability, Job: q.Job.Model, AttemptID: a.AttemptID, LeaseID: a.LeaseID, Token: a.Token, Epoch: a.Epoch, Revision: q.Job.Revision, ExpiresAt: d.cfg.Clock().Add(d.cfg.LeaseTTL), Hosted: r.Location == "hosted" || r.Ephemeral}
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.cfg.Transport.Deliver(d.life, delivery); err != nil && d.life.Err() == nil {
			d.lose(a, err) // a closing dispatcher leaves its leases to the expiry sweep
		}
	}()
}

// lose marks an undelivered lease lost; a lease that already moved on keeps its state.
func (d *Dispatcher) lose(a Admitted, cause error) {
	ctx := context.WithoutCancel(d.life)
	if state, epoch, err := d.cfg.Store.AttemptStateEpoch(ctx, a.AttemptID); err == nil && (state == "leased" || state == "running") {
		_ = d.cfg.Store.Transition(ctx, a.AttemptID, state, "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "infra", Reason: "delivery failed"})
	}
	if d.cfg.DeliveryError != nil {
		d.cfg.DeliveryError(a, cause)
	}
}
