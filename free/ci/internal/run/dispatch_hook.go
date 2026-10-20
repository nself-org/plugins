package run

// Purpose: registers the dispatcher (internal/dispatch, P7-SCHED-04) as the
// run.Dispatch hook, so every attempt CreateAttempt makes is placed and admitted
// through dispatch.Admit, the one admission chokepoint. This is the only file
// of the package that may import internal/dispatch (layers.yaml).
// Inputs:  an AttemptSpec and the run Session in the context.
// Outputs: an Admitted attempt, blocking until admitted, ctx done, E618 or E693.
// Constraints: the new behaviour exists only under compat.V15(); in v1.4 mode
// the hook is LocalAdmit, exactly as when no hook is registered. The world holds
// this machine and every registered node (dispatch_world.go); a remote placement
// is delivered by the dispatcher (NewTransportAdapter) and awaited by the run
// loop (remote_await.go). The local admission policy (battery, reservations, the
// foreground floor) still runs first, as in LocalAdmit.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

func init() {
	Dispatch = dispatchHook
	RegisterAfterRun(dispatchCleanup{})
}

// runDispatch is the dispatcher of one run and the facts its world needs.
type runDispatch struct {
	d    *dispatch.Dispatcher
	sess *Session
	row  store.PipelineRow
	polM sync.Mutex
	pol  *sched.Policy // resolved once; a failed resolution is retried, never cached
	qp   schedpolicy.QueuePolicy
	// spend is the spend policy resolved with pol; economics answers from it while the dispatcher runs.
	spend schedpolicy.SpendPolicy

	sink *remoteSink // what the carriers of this run's remote leases report to
}

var (
	runDispatchers sync.Map   // run id -> *runDispatch
	runDispatchMu  sync.Mutex // one construction at a time: OnStart hooks run once per dispatcher
)

// dispatchCleanup closes a run's dispatcher when the run is recorded.
type dispatchCleanup struct{}

func (dispatchCleanup) AfterRun(_ context.Context, r RunInfo) error {
	dispatchRelease(r.RunID)
	return nil
}

// dispatchRelease closes the dispatcher of a run. Coordinator.Run defers it, so a run that
// ends on an error path before AfterRun does not leave its dispatcher in the process map (D-1044).
func dispatchRelease(runID string) {
	if v, ok := runDispatchers.LoadAndDelete(runID); ok {
		rd := v.(*runDispatch)
		rd.d.Close()
		rd.sink.release()
		econRelease()
	}
}

func dispatchHook(ctx context.Context, s AttemptSpec) (Admitted, error) {
	if !compat.V15() {
		return LocalAdmit(ctx, s)
	}
	id, row, err := attemptIDs(ctx, s)
	if err != nil {
		return Admitted{}, err
	}
	sess := SessionFrom(ctx)
	mode := ciexec.Background
	if s.Foreground {
		mode = ciexec.Foreground
	}
	capacity := sess.Capacity()
	dec := ciexec.Admit(*s.Job, mode, capacity, sess.Admission)
	if !dec.Allowed {
		return Admitted{}, &AdmissionRefused{Reason: dec.Reason}
	}
	rd, err := runDispatcher(ctx, sess)
	if err != nil {
		return Admitted{}, err
	}
	cpu, mem := demandOf(s.Job)
	sj := rd.schedJob(s, id, row, cpu, mem)
	if s.N > 1 && ctx.Value(confirmingKey{}) == nil { // an infra retry avoids the runner that lost the previous attempt when it can
		if prev, e := sess.Store.LeasePreviousRunner(ctx, fmt.Sprintf("%s.a%d", row, s.N-1)); e == nil && prev != "" {
			_ = sess.Store.SetPreviousRunner(ctx, id, prev)
		}
	}
	rd.sink.expect(id, sess.Revision) // a remote carrier may report before this call returns
	a, err := rd.d.Submit(ctx, queueJob(sess, s, sj, id,
		max(dec.Slots*cpu, cpu), max(capacity.FreeMemoryMB-capacity.MemoryMB*dec.ReserveMemoryPercent/100, mem)))
	if err != nil || a.Local {
		rd.sink.forget(id)
	}
	if err != nil {
		return Admitted{}, mapDispatchError(err)
	}
	return Admitted{AttemptID: a.AttemptID, LeaseID: a.LeaseID, RunnerID: a.RunnerID, Local: a.Local}, nil
}

// queueJob is the dispatcher's input for one attempt. Only an interactive run
// (a person at a terminal) keeps the implicit foreground queue cap.
func queueJob(sess *Session, s AttemptSpec, sj sched.Job, id string, localCPU, localMemMB int) dispatch.Job {
	return dispatch.Job{AttemptID: id, Sched: sj, Model: *s.Job, Revision: sess.Revision, Foreground: s.Foreground,
		NoDefaultQueueCap: !sess.Interactive, LocalCPU: localCPU, LocalMemMB: localMemMB}
}

// mapDispatchError turns a dispatcher refusal (E618, E693, E694) into the
// run's error with the same code and message; anything else passes through.
func mapDispatchError(err error) error {
	var de *dispatch.Error
	if errors.As(err, &de) {
		return fail(de.Code, "%s", de.Message)
	}
	return err
}

// queue is the run's fairness and queue-depth policy (D9), resolved with the placement policy.
func (rd *runDispatch) queue(ctx context.Context) (dispatch.QueueLimits, error) {
	if _, err := rd.policy(ctx, ""); err != nil {
		return dispatch.QueueLimits{}, err
	}
	rd.polM.Lock()
	defer rd.polM.Unlock()
	return dispatch.QueueLimits{StarvationAfterMs: rd.qp.StarvationAfterMs, MaxDepth: rd.qp.MaxDepth}, nil
}

// runDispatcher returns the dispatcher of the session's run, creating it once.
func runDispatcher(ctx context.Context, sess *Session) (*runDispatch, error) {
	if v, ok := runDispatchers.Load(sess.RunID); ok {
		return v.(*runDispatch), nil
	}
	runDispatchMu.Lock()
	defer runDispatchMu.Unlock()
	if v, ok := runDispatchers.Load(sess.RunID); ok {
		return v.(*runDispatch), nil
	}
	rows, err := sess.Store.PipelinesByRevision(ctx, sess.Revision)
	if err != nil {
		return nil, err
	}
	rd := &runDispatch{sess: sess, sink: newRemoteSink(sess.Store)}
	for _, r := range rows {
		if r.ID == sess.RunID {
			rd.row = r
		}
	}
	if rd.row.ID == "" {
		return nil, fail(CodeRun, "run %s has no pipeline row", sess.RunID)
	}
	// The spend policy is resolved before the dispatcher starts and economics is bound to it:
	// the admission step answers from memory, and an unbound process refuses metered and ephemeral runners.
	if _, err = rd.policy(ctx, ""); err != nil {
		return nil, err
	}
	econAcquire(sess.Store, sess.Host, rd.spendPolicy)
	rd.d, err = dispatch.New(dispatch.Config{Store: sess.Store, Host: sess.Host, World: rd.world, Poll: sess.Poll, LeaseTTL: RemoteLeaseTTL, Queue: rd.queue,
		Slots:         func() int { return rd.sess.Capacity().CPUs },
		Incarnation:   func() int64 { return highestIncarnation(rd.sess.Store) },
		PendingSpend:  notePending,
		Transport:     newTransportAdapter(rd.sink, rd.sink, rd.row.Project),
		DeliveryError: func(a dispatch.Admitted, err error) { rd.sink.ended(a.AttemptID, err) }})
	if err != nil {
		econRelease()
		return nil, err
	}
	runDispatchers.Store(sess.RunID, rd)
	return rd, nil
}

// NewTransportAdapter adapts internal/transport to the dispatcher's Transport:
// Known looks the driver up by kind, Deliver builds the protocol lease and runs
// the preparers and the driver (transport.Deliver). sink receives the carrier's
// output.
func NewTransportAdapter(sink transport.Sink) dispatch.Transport {
	return newTransportAdapter(sink, nil, "")
}

// carriers is what the adapter tells the run about the carriers it starts: the
// run cancels one, and learns when it has ended.
type carriers interface {
	carrierStart(ctx context.Context, attemptID, leaseID string) (context.Context, func())
}

func newTransportAdapter(sink transport.Sink, reg carriers, root string) dispatch.Transport {
	return transportAdapter{sink: sink, reg: reg, root: root}
}

type transportAdapter struct {
	sink transport.Sink
	reg  carriers
	root string // the repository whose revision a node receives
}

func (transportAdapter) Known(kind string) bool {
	_, ok := transport.Lookup(kind)
	return ok
}

func (a transportAdapter) Deliver(ctx context.Context, d dispatch.Delivery) error {
	if a.reg != nil {
		var end func()
		ctx, end = a.reg.carrierStart(ctx, d.AttemptID, d.LeaseID)
		defer end()
	}
	if RemoteRepo != nil && a.root != "" {
		RemoteRepo(a.root)
	}
	lease := protocol.Lease{LeaseID: d.LeaseID, AttemptID: d.AttemptID, Epoch: uint64(d.Epoch), Token: d.Token, CompatMode: protocol.Compat15, Job: d.Job,
		Checkout: protocol.Checkout{Revision: d.Revision}, Secrets: []protocol.SecretRef{}, ExpiresAt: d.ExpiresAt.UTC().Format(time.RFC3339)}
	return transport.Deliver(ctx, d.Node, lease, a.sink)
}

// highestIncarnation is the incarnation fenced leases carry: the highest of the coordinator table, so a
// leased epoch after a `serve restore` exceeds every epoch granted before the backup (P7-NODE-20).
func highestIncarnation(st *store.Store) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, _ := st.HighestIncarnation(ctx) // a failed read is 1, the old behaviour
	return n
}
