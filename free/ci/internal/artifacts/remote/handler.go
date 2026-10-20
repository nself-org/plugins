// Package remote is the coordinator side of artifacts from remote attempts
// (Epic CACHE D4, D7): it admits the blobs a node uploads over the runner
// protocol (digest, chunk) into the project's artifact realm and turns each
// `artifact-ref` into a staged record, so the attempt's finalizing guard
// (records.CommitAndVerify) sees digests it can verify.
//
// Inputs:  a Binding per delivered lease (attempt, lease id, lease epoch, job)
// and the frames the transport Sink receives for it.
// Outputs: blobs in the realm, staged artifact records, E714 and E712 refusals,
// one audit row per blob refused for a stale epoch.
// Constraints: a blob is written only while the lease epoch is current and the
// attempt is leased or running; the realm is the attempt's project, taken from
// the store, never from the frame; a ref is accepted only for a path some
// declaration of the attempt selects and only for a blob the realm holds at
// that size; no listener, no process, no SQL (the Ledger is the store).
package remote

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Ledger is the part of the store the handler uses (*store.Store).
type Ledger interface {
	records.Ledger
	AttemptStateEpoch(ctx context.Context, id string) (model.JobState, int64, error)
	AppendAudit(ctx context.Context, actor, action, target, detail string) error
}

// Handler holds what every lease shares.
type Handler struct {
	Ledger Ledger
	Blobs  *cas.Store
	Policy records.Policy
	Now    func() time.Time
}

// Binding is one delivered lease.
type Binding struct {
	AttemptID string
	LeaseID   string
	// Epoch is the lease epoch the lease was delivered under.
	Epoch int64
	Job   model.Job
	// Fence returns nil while Epoch is the lease's current epoch (lease.Engine.Fence).
	Fence func(ctx context.Context) error
	// Ready moves a leased attempt to running (the first sign of life); nil when nothing is needed.
	Ready func(ctx context.Context) error
}

// Bind returns the per-lease handler.
func (h *Handler) Bind(b Binding) *Lease { return &Lease{h: h, b: b} }

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}
