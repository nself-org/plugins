package run

// Purpose: registers the dispatcher (internal/dispatch, P7-SCHED-04) as the
// run.Dispatch hook, so every attempt CreateAttempt makes is placed and admitted
// through dispatch.Admit, the one admission chokepoint. This is the only file
// of the package that may import internal/dispatch (layers.yaml).
// Inputs:  an AttemptSpec and the run Session in the context.
// Outputs: an Admitted attempt, blocking until admitted, ctx done, E618 or E693.
// Constraints: the new behaviour exists only under compat.V15(); in v1.4 mode
// the hook is LocalAdmit, exactly as when no hook is registered. This
// coordinator awaits only local attempts (runAttempt fails a remote one), so
// the world holds the local runner alone until a remote await exists; the
// dispatcher itself delivers remote leases (NewTransportAdapter). The local
// runner carries a PLACEHOLDER declaration at the top of each trust axis (debt:
// not a measured fact; `ci explain` shows it as declared): the isolation hook
// refuses a job it cannot sandbox at exec time, and P7-TRUST-08's probe facts
// replace it. The local admission policy (battery, reservations, the foreground
// floor) still runs first, as in LocalAdmit.

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
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
}

var (
	runDispatchers sync.Map   // run id -> *runDispatch
	runDispatchMu  sync.Mutex // one construction at a time: OnStart hooks run once per dispatcher
)

// dispatchCleanup closes a run's dispatcher when the run is recorded.
type dispatchCleanup struct{}

func (dispatchCleanup) AfterRun(_ context.Context, r RunInfo) error {
	if v, ok := runDispatchers.LoadAndDelete(r.RunID); ok {
		v.(*runDispatch).d.Close()
	}
	return nil
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
	a, err := rd.d.Submit(ctx, dispatch.Job{
		AttemptID: id, Sched: rd.schedJob(s, id, row, cpu, mem), Model: *s.Job, Revision: sess.Revision, Foreground: s.Foreground,
		LocalCPU: max(dec.Slots*cpu, cpu), LocalMemMB: max(capacity.FreeMemoryMB-capacity.MemoryMB*dec.ReserveMemoryPercent/100, mem),
	})
	if err != nil {
		var de *dispatch.Error
		if errors.As(err, &de) {
			return Admitted{}, fail(de.Code, "%s", de.Message)
		}
		return Admitted{}, err
	}
	return Admitted{AttemptID: a.AttemptID, LeaseID: a.LeaseID, RunnerID: a.RunnerID, Local: a.Local}, nil
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
	rd := &runDispatch{sess: sess}
	for _, r := range rows {
		if r.ID == sess.RunID {
			rd.row = r
		}
	}
	if rd.row.ID == "" {
		return nil, fail(CodeRun, "run %s has no pipeline row", sess.RunID)
	}
	rd.d, err = dispatch.New(dispatch.Config{Store: sess.Store, Host: sess.Host, World: rd.world, Poll: sess.Poll})
	if err != nil {
		return nil, err
	}
	runDispatchers.Store(sess.RunID, rd)
	return rd, nil
}

// platform is the local runner's platform string.
func platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

func (rd *runDispatch) schedJob(s AttemptSpec, id, row string, cpu, mem int) sched.Job {
	var timeout time.Duration
	if d, err := time.ParseDuration(s.Job.Timeout); err == nil {
		timeout = d
	}
	return sched.Job{Key: id, ID: row, Name: row, Pipeline: sched.Pipeline{ID: rd.sess.RunID, Support: []string{platform()}, Foreground: s.Foreground},
		Requirements: model.PlacementRequirements{Platform: platform(), CPU: int64(cpu), MemMB: int64(mem)},
		Trust:        model.TrustClass(rd.row.SourceTrust), Network: "internet", TimeoutMs: timeout.Milliseconds(), Attempt: s.N, Priority: string(s.Job.Priority),
		RemoteOnly: s.Job.RemoteOnly, PersonalOwnRun: true, Project: rd.row.Project}
}

// policy resolves the run's effective scheduling policy once, as PolicyHook does.
// A failure is returned and retried on the next call: a transient git or store
// error must not fail every later job of the run.
func (rd *runDispatch) policy(ctx context.Context, _ string) (sched.Policy, error) {
	rd.polM.Lock()
	defer rd.polM.Unlock()
	if rd.pol != nil {
		return *rd.pol, nil
	}
	root := rd.row.Project
	if root == "" {
		root = "."
	}
	res := policy.Resolver{Store: rd.sess.Store, Run: policy.Runner(report.Capture)}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGit(ctx, res.Run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		return sched.Policy{}, err
	}
	p, err := schedpolicy.Placement(&got.Effective)
	if err != nil {
		return sched.Policy{}, err
	}
	rd.pol = &p
	return p, nil
}

// world builds the local-only placement world of the run.
func (rd *runDispatch) world(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
	capacity := rd.sess.Capacity()
	local := localCapability(rd.row.Project, capacity)
	w, _, err := world.Build(ctx, world.Deps{Store: rd.sess.Store, Local: &local, LocalCapacity: &capacity, Policy: rd.policy}, p)
	if err != nil {
		return sched.World{}, "", err
	}
	keep := w.Runners[:0]
	for _, r := range w.Runners {
		if r.ID == "local" {
			r.Decl = localDeclarationPlaceholder()
			keep = append(keep, r)
		}
	}
	if len(keep) != 1 {
		// a registered node called "local" would inherit the placeholder declaration
		return sched.World{}, "", fail(CodeOptions, "the node registry holds a node with the reserved id %q", "local")
	}
	w.Runners, w.Hosted = keep, nil
	digest, err := world.Digest(w)
	return w, digest, err
}

func localCapability(project string, c ciexec.Capacity) model.Capability {
	osName, arch, kind, class := runtime.GOOS, runtime.GOARCH, "local", "local"
	cap := model.Capability{Schema: "ci.runner-capability/v1"}
	cap.Lifecycle.Eligible = true
	cap.Lifecycle.AuthorizedProjects = []string{project}
	cap.Protocol.Versions, cap.Protocol.Min = []int{1}, 1
	cap.Platform.OS.Value, cap.Platform.Arch.Value = &osName, &arch
	cap.Location.Kind.Value = &kind
	cap.Economics.Class.Value = &class
	return cap
}

// localDeclarationPlaceholder is what the PERSONAL coordinator says about its own
// machine until a probe measures it. It is permissive on purpose: placement must
// not refuse what LocalAdmit ran, and the exec-time isolation hook is the gate.
func localDeclarationPlaceholder() trust.Declaration {
	return trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged",
		UIDSeparation: true, Ownership: "operator", PersonalOwnNode: true, NoLANRoute: true}
}

// NewTransportAdapter adapts internal/transport to the dispatcher's Transport:
// Known looks the driver up by kind, Deliver builds the protocol lease and runs
// the preparers and the driver (transport.Deliver). sink receives the carrier's
// output.
func NewTransportAdapter(sink transport.Sink) dispatch.Transport { return transportAdapter{sink} }

type transportAdapter struct{ sink transport.Sink }

func (transportAdapter) Known(kind string) bool {
	_, ok := transport.Lookup(kind)
	return ok
}

func (a transportAdapter) Deliver(ctx context.Context, d dispatch.Delivery) error {
	lease := protocol.Lease{LeaseID: d.LeaseID, AttemptID: d.AttemptID, Epoch: uint64(d.Epoch), Token: d.Token, CompatMode: protocol.Compat15, Job: d.Job,
		Checkout: protocol.Checkout{Revision: d.Revision}, Secrets: []protocol.SecretRef{}, ExpiresAt: d.ExpiresAt.UTC().Format(time.RFC3339)}
	return transport.Deliver(ctx, d.Node, lease, a.sink)
}
