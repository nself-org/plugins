package dispatch

// Purpose: dispatch.Admit, the one admission chokepoint (Epic D7, R-SEC-22).
// Every lease and every local attempt is created here: Admit runs
// store.AdmitPlacement, which re-reads the counters and the host reservation,
// runs the registered steps in name order and commits the lease with them in
// one transaction.
// Inputs:  a store and a Request naming the attempt, its placement and limits.
// Outputs: an Admitted attempt (leased), or a *Refusal (stays queued, reason
// recorded), ErrConflict (state moved; re-plan next tick) or a store error.
// Constraints: nothing else in the module may call store.AdmitPlacement,
// store.AdmitAndLease or lease.Grant (TestAdmitChokepoint); a placement computed
// from an older world is safe because the counters and reservations are re-read
// inside the transaction.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ErrConflict means the attempt changed under the admission (E605): re-plan.
var ErrConflict = errors.New("dispatch: attempt state changed; re-plan")

// Request is one placement to admit.
type Request struct {
	AttemptID string
	Placement sched.Placement
	Runner    sched.Runner
	Job       sched.Job
	// Local marks the placement for this machine: no fence, no delivery.
	Local bool
	// Host is the reservation scope: this machine for a local runner, the node
	// for a remote one (X10).
	Host                       string
	CapacityCPU, CapacityMemMB int
	Limits                     sched.Limits
	RunnerSlots, ProviderLimit int
	Foreground                 bool
	WorldDigest                string
	// LeaseTTL is the heartbeat TTL of a remote lease (0 = the store default).
	LeaseTTL time.Duration
	// Incarnation is the coordinator incarnation of a fenced remote lease.
	Incarnation int64
}

// Admitted is an attempt that holds a lease.
type Admitted struct {
	AttemptID, LeaseID, RunnerID string
	Local                        bool
	// Epoch is the fencing epoch of a remote lease; Token is its secret, never stored.
	Epoch int64
	Token string
}

// Admit admits one placement; see the file comment.
func Admit(ctx context.Context, st *store.Store, r Request) (Admitted, error) {
	if st == nil || r.AttemptID == "" || r.Placement.RunnerID == "" || r.Host == "" {
		return Admitted{}, fail(CodeUnplaceable, "admission needs a store, an attempt, a runner and a host")
	}
	state, epoch, err := st.AttemptStateEpoch(ctx, r.AttemptID)
	if err != nil {
		return Admitted{}, err
	}
	if state != "queued" {
		return Admitted{}, ErrConflict
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return Admitted{}, err
	}
	digest := sha256.Sum256(token)
	cpu, mem := int(max(r.Placement.CPU, 1)), int(max(r.Placement.MemMB, 256))
	d := store.Demand{AttemptID: r.AttemptID, RunnerID: r.Placement.RunnerID, LeaseID: "lease-" + r.AttemptID, TokenDigest: hex.EncodeToString(digest[:]),
		CPU: cpu, MemMB: mem, CapacityCPU: max(r.CapacityCPU, 0), CapacityMemMB: max(r.CapacityMemMB, 0), Epoch: epoch}
	if !r.Local {
		d.TTL = r.LeaseTTL
	}
	req := store.AdmitRequest{User: r.Job.User, Team: r.Job.Team, Provider: r.Runner.Provider, WorldDigest: r.WorldDigest}
	if !r.Local {
		req.Incarnation = max(r.Incarnation, 1)
	}
	ordered := seal()
	stored := make([]store.AdmitStep, 0, len(ordered))
	for _, n := range ordered {
		stored = append(stored, func(ctx context.Context, v store.AdmitView) error {
			return n.step(ctx, View{AdmitView: v, Request: r})
		})
	}
	fence, err := st.AdmitPlacement(ctx, r.Host, d, req, stored...)
	if err != nil {
		return Admitted{}, classify(ctx, st, r.AttemptID, err)
	}
	a := Admitted{AttemptID: r.AttemptID, LeaseID: d.LeaseID, RunnerID: d.RunnerID, Local: r.Local, Epoch: fence}
	if !r.Local {
		a.Token = hex.EncodeToString(token)
	}
	return a, nil
}

// refuse records why the attempt stays queued; the record is best effort
// because the refusal itself already stands.
func refuse(ctx context.Context, st *store.Store, attemptID string, ref *Refusal) *Refusal {
	ctx = context.WithoutCancel(ctx)
	if reason, detail, ok, err := st.Refusal(ctx, attemptID); err != nil || !ok || reason != string(ref.Reason) || detail != ref.Detail {
		_ = st.RecordRefusal(ctx, attemptID, string(ref.Reason), ref.Detail)
	}
	return ref
}

func classify(ctx context.Context, st *store.Store, attemptID string, err error) error {
	var ref *Refusal
	var se *store.Error
	switch {
	case errors.As(err, &ref):
		return refuse(ctx, st, attemptID, ref)
	case errors.As(err, &se) && se.Code == "E605":
		return ErrConflict
	case errors.As(err, &se) && se.Code == "E604" && se.Message == "host capacity exhausted":
		return refuse(ctx, st, attemptID, &Refusal{Reason: model.CapacityBusy, Detail: "host reservation is full"})
	}
	return err
}
