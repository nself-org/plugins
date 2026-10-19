package drain

// Purpose: EngineLeases adapts P7-NODE-09's lease engine (internal/lease) to the drain's Leases interface:
//          listing running leases, re-reading one, handing back after an expiry (LeaseFail on that lease only)
//          and moving a release job to needs-operator.
// Inputs:  the lease Engine, this coordinator's id and JobInfo, which says the kind and idempotence of an attempt.
// Outputs: Lease values and the store transitions the engine owns.
// Constraints: fail closed: without JobInfo Running is an error, and a lease whose JobInfo fails has kind
//          "unknown": it is neither handed back nor moved (the store would requeue an idempotent job
//          while its agent may still run it), the drain reports an error and restart grace decides; for
//          a release or idempotent:false attempt the store routes LeaseFail to needs-operator.

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// EngineLeases is the Leases implementation over the lease engine.
type EngineLeases struct {
	Engine        *lease.Engine
	CoordinatorID string // only leases of this coordinator are drained; "" means all
	// Owned, when set, decides whether a coordinator id belongs to this process; leases of other
	// coordinators sharing the store (another `ci` process) are never cancelled or handed back.
	Owned func(ctx context.Context, coordinatorID string) bool
	// JobInfo returns the job kind and idempotence of an attempt; nil fails closed.
	JobInfo func(ctx context.Context, attemptID string) (kind string, idempotent bool, err error)
}

// KindUnknown marks a lease whose job class could not be read.
const KindUnknown = "unknown"

func (e *EngineLeases) lease(ctx context.Context, rl store.RemoteLease) Lease {
	l := Lease{ID: rl.ID, AttemptID: rl.AttemptID, NodeID: rl.RunnerID, Epoch: rl.Epoch, Local: rl.Epoch <= 0, ExpiresAt: time.Unix(0, rl.ExpiresAt),
		Kind: KindUnknown, Idempotent: false}
	if kind, idem, err := e.JobInfo(ctx, rl.AttemptID); err == nil {
		l.Kind, l.Idempotent = kind, idem
	}
	return l
}

// Running lists the leased and running leases of this process's coordinators. A local in-process lease
// (epoch 0) is listed so the drain waits for its job, but it is only waited for (Lease.Local).
func (e *EngineLeases) Running(ctx context.Context) ([]Lease, error) {
	if e.JobInfo == nil {
		return nil, errors.New("drain: EngineLeases needs JobInfo")
	}
	all, err := e.Engine.Store.LeaseExpired(ctx, time.Unix(0, math.MaxInt64)) // every lease whose attempt is leased or running
	if err != nil {
		return nil, err
	}
	var out []Lease
	for _, rl := range all {
		if (e.CoordinatorID == "" || rl.CoordinatorID == e.CoordinatorID) && (e.Owned == nil || e.Owned(ctx, rl.CoordinatorID)) {
			out = append(out, e.lease(ctx, rl))
		}
	}
	return out, nil
}

// Get re-reads one lease.
func (e *EngineLeases) Get(ctx context.Context, id string) (Lease, bool, error) {
	if e.JobInfo == nil {
		return Lease{}, false, errors.New("drain: EngineLeases needs JobInfo")
	}
	rl, err := e.Engine.Store.LeaseGet(ctx, id)
	if err != nil {
		var coded *store.Error
		if errors.As(err, &coded) && coded.Code == "E662" {
			return Lease{}, false, nil
		}
		return Lease{}, false, err
	}
	if rl.State != "leased" && rl.State != "running" {
		return Lease{}, false, nil
	}
	return e.lease(ctx, rl), true, nil
}

// HandBack requeues the lease's job: LeaseFail after expiry, Reconnect(self_fenced) for an agent-reported fence.
// The drain itself hands back only after an expiry: a cancel ack means the agent received the cancel, not
// that its job and after-job hooks stopped.
func (e *EngineLeases) HandBack(ctx context.Context, l Lease, expired bool, _ string) error {
	if expired { // this lease only: Sweep would also fail every other expired lease of the store
		rl, err := e.Engine.Store.LeaseGet(ctx, l.ID)
		if err != nil {
			if hasStoreCode(err, "E662") {
				return nil // the lease is gone: its job ended meanwhile
			}
			return err
		}
		if err = e.Engine.Store.LeaseFail(ctx, rl, e.Engine.Config.Clock(), "lease.expired", true); err != nil {
			if hasStoreCode(err, "E605") {
				return nil // renewed or ended meanwhile
			}
			return err
		}
		return nil
	}
	return e.Engine.Reconnect(ctx, l.ID, l.Epoch, "self_fenced")
}

// NeedsOperator ends the lease without a retry; the store sends release and idempotent:false attempts to
// needs-operator and nothing else is queued.
func (e *EngineLeases) NeedsOperator(ctx context.Context, l Lease, reason string) error {
	if l.HandBackable() {
		return errors.New("drain: lease may be handed back; refusing needs-operator")
	}
	if l.Kind == KindUnknown {
		return errors.New("drain: job class unknown; the lease is left to restart grace")
	}
	rl, err := e.Engine.Store.LeaseGet(ctx, l.ID)
	if err != nil {
		return err
	}
	return e.Engine.Store.LeaseFail(ctx, rl, e.Engine.Config.Clock(), reason, false)
}

func hasStoreCode(err error, code string) bool {
	var coded *store.Error
	return errors.As(err, &coded) && coded.Code == code
}
