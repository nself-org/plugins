//go:build security

package security

// Purpose: shared fixtures of the security acceptance suite (P7-TRUST-17): a private
// store, queued attempts, LAN and hosted runners, the effective policy of a repository
// and a real dispatcher over them, with a hosted-provider fake behind the transport.
// Inputs:  none. Outputs: helpers only.
// Constraints: tests only; no network beyond 127.0.0.1; every value is synthetic; a
// missing prerequisite is a t.Fatal, never a skip.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/fake"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func ptr[T any](v T) *T { return &v }

func openStore(t testing.TB) *store.Store {
	t.Helper()
	return openStoreAt(t, filepath.Join(t.TempDir(), "state.db"))
}

func openStoreAt(t testing.TB, path string) *store.Store {
	t.Helper()
	s, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var rowSeq atomic.Int64

// queueAttempt creates pipeline pipe (project proj, source class srcTrust), a job
// named name and its queued attempt, and returns the attempt id.
func queueAttempt(t testing.TB, st *store.Store, pipe, proj, name, srcTrust string) string {
	t.Helper()
	ctx := context.Background()
	err := st.CreatePipeline(ctx, store.PipelineRow{ID: pipe, Revision: "rev", Project: proj, Trigger: "local", SourceTrust: srcTrust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatal(err)
	}
	id := pipe + "." + name + "." + strconv.FormatInt(rowSeq.Add(1), 10)
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: pipe, Name: name, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	return id
}

// lanRunner is a LAN node that accepts every class, can reach the LAN and offers a VM.
func lanRunner(id string, slots int) sched.Runner {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Lifecycle.AuthorizedProjects = []string{"project"}
	c.Lifecycle.Eligible = true
	c.Protocol.Versions, c.Protocol.Min = []int{1}, 1
	c.Platform.OS.Value, c.Platform.Arch.Value = ptr("linux"), ptr("amd64")
	c.Resources.CPU.Value, c.Resources.MemMB.Value = ptr(float64(8)), ptr(int64(8192))
	c.Availability.Slots.Value = &slots
	c.Identity.ID, c.Identity.Provider, c.Identity.Transport = id, "operator", "kind-"+id
	c.Identity.Labels = []string{"arch=amd64"}
	loc := "lan"
	c.Location.Kind.Value = &loc
	return sched.Runner{ID: id, Location: loc, Class: "owned", Provider: "operator", Avail: sched.Avail{Slots: slots, CPU: 8, MemMB: 8192}, Capability: c,
		Decl: trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "vm", Network: "privileged", UIDSeparation: true, Ownership: "operator", PersonalOwnNode: false, NoLANRoute: false}}
}

// hostedRunner is the hosted-disposable fake provider's capacity: one disposable
// runner with no route to any LAN.
func hostedRunner(id string) sched.Runner {
	r := lanRunner(id, 4)
	loc := "hosted"
	r.Location, r.Class, r.Provider = loc, "included", "fake"
	r.Capability.Location.Kind.Value = &loc
	r.Capability.Identity.Provider = "fake"
	// Hosted capacity for public repositories is free, so no allowance gates it: only the
	// privacy rule stands between a private job and this runner.
	r.Capability.Identity.Labels = append(r.Capability.Identity.Labels, "nself.visibility=public")
	r.Decl = trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: "hosted-disposable", Network: "internet", UIDSeparation: true, Ownership: "provider", Hosted: true, Provider: "fake", NoLANRoute: true}
	return r
}

// gvisorRunner is a LAN-less Linux node with gVisor, the only non-hosted home of untrusted code.
func gvisorRunner(id string, slots int) sched.Runner {
	r := lanRunner(id, slots)
	r.Decl.Isolation, r.Decl.Network, r.Decl.NoLANRoute = "sandboxed-container", "restricted", true
	return r
}

func newJob(attempt string, class model.TrustClass, net model.NetworkScope, secrets ...model.SecretClass) sched.Job {
	return sched.Job{Key: attempt, ID: attempt, Name: attempt, Pipeline: sched.Pipeline{ID: "pipe", Support: []string{"linux/amd64"}},
		Requirements: model.PlacementRequirements{Platform: "linux/amd64", CPU: 1, MemMB: 128}, Trust: class, Network: net, SecretClasses: secrets, TimeoutMs: 60000, Project: "project"}
}

// registerNode puts a remote runner in the node registry as online, the way
// world.Build finds it; admission re-reads that row. Idempotent.
func registerNode(st *store.Store, r sched.Runner) {
	c, err := planview.LocalCapability(planview.LocalFacts{Project: "project", Platform: "linux/amd64", CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		panic(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = r.ID, r.ID, "ssh"
	doc, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	_ = st.NodePut(context.Background(), store.NodeRecord{ID: r.ID, Name: r.ID, State: "online", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test")
}

// repoPolicy resolves the effective policy of a repository the way the dispatcher
// feeds it to the scheduler: defaults by visibility and source class, no operator rows.
func repoPolicy(t testing.TB, visibility string, class model.TrustClass) trust.Effective {
	t.Helper()
	r := policy.Resolver{Run: policy.Runner(report.Capture)}
	got, err := r.Resolve(context.Background(), policy.Repo{Root: t.TempDir(), Project: "project", Visibility: visibility, Class: class}, policy.Revision{})
	if err != nil {
		t.Fatal(err)
	}
	return got.Effective
}

// worldOf builds the world from the store's reservations so successive ticks see each
// other. Hosted runners go to the hosted pool with an allowance, LAN nodes are registered.
func worldOf(st *store.Store, eff trust.Effective, runners ...sched.Runner) func(context.Context, sched.Pipeline) (sched.World, string, error) {
	return func(ctx context.Context, _ sched.Pipeline) (sched.World, string, error) {
		snap, err := st.WorldSnapshot(ctx)
		if err != nil {
			return sched.World{}, "", err
		}
		w := sched.World{Policy: sched.Policy{Preset: "balanced", Trust: eff}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}, Allowances: map[string]sched.Allowance{},
			Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}}
		for _, r := range runners {
			if r.Location != "local" && r.Location != "hosted" {
				registerNode(st, r)
			}
			rs := snap.Reservations[r.ID]
			r.Avail.Slots = max(0, *r.Capability.Availability.Slots.Value-int(rs.Slots))
			r.Avail.CPU -= rs.CPU
			r.Avail.MemMB -= rs.MemMB
			if r.Location == "hosted" {
				w.Hosted = append(w.Hosted, r)
				w.Allowances[r.Provider] = sched.Allowance{RemainingEstimated: 1 << 50, Known: true, CeilingKnown: true}
			} else {
				w.Runners = append(w.Runners, r)
			}
		}
		return w, "digest-" + strconv.FormatInt(time.Now().UnixNano(), 10), nil
	}
}

type recSink struct {
	mu  sync.Mutex
	got []dispatch.Decision
}

func (r *recSink) Record(_ context.Context, d dispatch.Decision) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, d)
}

func (r *recSink) all() []dispatch.Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]dispatch.Decision(nil), r.got...)
}

// last returns the newest decision recorded for the job key, if any.
func (r *recSink) last(key string) (dispatch.Decision, bool) {
	all := r.all()
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Job.Key == key {
			return all[i], true
		}
	}
	return dispatch.Decision{}, false
}

// nullSink is the transport.Sink behind the fake drivers: nothing is stored.
type nullSink struct{}

func (nullSink) Log(context.Context, protocol.Log) error                 { return nil }
func (nullSink) Result(context.Context, protocol.Result) error           { return nil }
func (nullSink) ArtifactRef(context.Context, protocol.ArtifactRef) error { return nil }
func (nullSink) CacheRef(context.Context, protocol.CacheRef) error       { return nil }
func (nullSink) Blob(context.Context, transport.BlobHeader) (transport.BlobWriter, error) {
	return nil, fmt.Errorf("unused")
}

// currentHosted is the hosted-provider fake of the running test: the fake driver of a hosted
// runner hands its lease to it, the way the real hosted driver hands one to its provider.
var currentHosted atomic.Pointer[fake.Fake]

// fakeDriver is a transport driver of the NODE-02 registry (P7-NODE-02): it accepts a lease and
// returns. A lease for a hosted-provider runner goes to the provider fake's Dispatch.
type fakeDriver struct{ kind string }

func (d fakeDriver) Kind() string { return d.kind }
func (fakeDriver) Run(ctx context.Context, node model.Capability, lease protocol.Lease, _ transport.Sink) error {
	if node.Identity.Provider == "fake" {
		if h := currentHosted.Load(); h != nil {
			_, err := h.Dispatch(ctx, providers.Attempt{ID: lease.AttemptID, N: 1, Project: "project", Revision: lease.Checkout.Revision, Trust: "untrusted"})
			return err
		}
	}
	return nil
}

var registered sync.Map

// registerKinds registers a fake driver for the transport kind of each runner, once per process.
func registerKinds(t testing.TB, runners ...sched.Runner) {
	t.Helper()
	for _, r := range runners {
		kind := r.Capability.Identity.Transport
		if _, loaded := registered.LoadOrStore(kind, true); loaded {
			continue
		}
		if err := transport.Register(fakeDriver{kind}); err != nil {
			t.Fatal(err)
		}
	}
}

// fabricTransport records every delivery and hands it to the real transport adapter, which looks the
// driver up in the NODE-02 registry (the fakes above).
type fabricTransport struct {
	inner dispatch.Transport
	mu    sync.Mutex
	sent  []dispatch.Delivery
}

func newFabricTransport() *fabricTransport {
	return &fabricTransport{inner: run.NewTransportAdapter(nullSink{})}
}

func (f *fabricTransport) Known(kind string) bool { return f.inner.Known(kind) }

func (f *fabricTransport) Deliver(ctx context.Context, d dispatch.Delivery) error {
	f.mu.Lock()
	f.sent = append(f.sent, d)
	f.mu.Unlock()
	return f.inner.Deliver(ctx, d)
}

func (f *fabricTransport) deliveries() []dispatch.Delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dispatch.Delivery(nil), f.sent...)
}

// rig is a real dispatcher over a real store with the hosted fake behind it.
type rig struct {
	t      *testing.T
	db     string
	st     *store.Store
	d      *dispatch.Dispatcher
	sink   *recSink
	tr     *fabricTransport
	hosted *fake.Fake
}

func newRig(t *testing.T, eff trust.Effective, runners ...sched.Runner) *rig {
	t.Helper()
	db := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, db)
	hosted := fake.New(fake.Options{Token: "synthetic-provider-token"})
	registerKinds(t, runners...)
	currentHosted.Store(hosted)
	t.Cleanup(func() { currentHosted.Store(nil) })
	r := &rig{t: t, db: db, st: st, sink: &recSink{}, tr: newFabricTransport(), hosted: hosted}
	d, err := dispatch.New(dispatch.Config{Store: st, Host: "host", Sink: r.sink, Transport: r.tr, World: worldOf(st, eff, runners...), Poll: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	r.d = d
	return r
}

// prepare creates the pipeline (run "pipe-<key>"), job and queued attempt of job and returns the
// attempt id; the job's key becomes the attempt id.
func (r *rig) prepare(job sched.Job, srcTrust string) (string, sched.Job) {
	r.t.Helper()
	id := queueAttempt(r.t, r.st, "pipe-"+job.Key, "project", job.Name, srcTrust)
	job.Key = id
	return id, job
}

// enqueue submits a prepared attempt and waits up to wait for admission: the lease, or the
// context error when the job stayed queued (the sink then holds why).
func (r *rig) enqueue(id string, job sched.Job, wait time.Duration) (dispatch.Admitted, error) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	return r.d.Submit(ctx, dispatch.Job{AttemptID: id, Sched: job, Revision: strings.Repeat("a", 40)})
}

// submit prepares and enqueues in one step.
func (r *rig) submit(job sched.Job, srcTrust string, wait time.Duration) (string, dispatch.Admitted, error) {
	r.t.Helper()
	id, job := r.prepare(job, srcTrust)
	a, err := r.enqueue(id, job, wait)
	return id, a, err
}

// hostedSawRequest reports whether the provider fake received any dispatch. The fake keeps
// one header map for all requests: it stays empty until the first Dispatch sets Authorization.
func hostedSawRequest(f *fake.Fake) bool {
	for _, h := range f.Requests() {
		if len(h) > 0 {
			return true
		}
	}
	return false
}

// pass walks an admitted attempt to a terminal state so its capacity frees.
func pass(t testing.TB, st *store.Store, id string) {
	t.Helper()
	ctx := context.Background()
	for _, step := range [][2]model.JobState{{"leased", "running"}, {"running", "finalizing"}, {"finalizing", "passed"}} {
		_, epoch, err := st.AttemptStateEpoch(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err = st.Transition(ctx, id, step[0], step[1], store.TransitionDetail{Epoch: epoch}); err != nil {
			t.Fatal(err)
		}
	}
}
