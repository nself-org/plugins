package run

// Purpose: P7-AUTO-28 unit tests of the remote sink and the placement inputs of a
// run that has registered nodes: log buffering, fenced results, loss, carrier
// cancel, lease renewal, the hook's world, the secret rule and plan/run parity.
// Inputs:  a private store, a leased remote attempt, fake carriers.
// Outputs: assertions on the store states and the channels of the rendezvous.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

type remoteEnv struct {
	st    *store.Store
	sink  *remoteSink
	id    string
	lease string
	epoch int64
}

// newRemoteEnv leases attempt run1.jobA.a1 on node n1 through the lease engine.
func newRemoteEnv(t *testing.T) *remoteEnv {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	row := store.PipelineRow{ID: "run1", Project: "/proj", Revision: "rev", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(ctx, row); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateJob(ctx, store.JobRow{ID: "run1.jobA", PipelineID: "run1", Name: "jobA", Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	e := &remoteEnv{st: st, sink: newRemoteSink(st), id: "run1.jobA.a1"}
	e.lease = "lease-" + e.id
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: e.id, JobID: "run1.jobA", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	_, epoch, err := st.AttemptStateEpoch(ctx, e.id)
	if err != nil {
		t.Fatal(err)
	}
	e.epoch, err = e.sink.eng.Grant(ctx, "host", store.Demand{AttemptID: e.id, RunnerID: "n1", LeaseID: e.lease, TokenDigest: "d", CPU: 1, MemMB: 1, CapacityCPU: 8, CapacityMemMB: 1024, TTL: time.Minute, Epoch: epoch}, 1)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *remoteEnv) state(t *testing.T) string {
	t.Helper()
	s, _, err := e.st.AttemptStateEpoch(context.Background(), e.id)
	if err != nil {
		t.Fatal(err)
	}
	return string(s)
}

const testRev = "0123456789abcdef0123456789abcdef01234567"

func evJSON(result string) json.RawMessage {
	raw, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Revision: testRev, Attempt: &model.AttemptRef{ID: "run1.jobA.a1", N: 1}, Result: model.Result(result)})
	return raw
}

func TestRemoteAwaitLogsAreBufferedUntilTheRunAttaches(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	ctx := context.Background()
	for _, l := range []string{"one", "two\n"} {
		if err := e.sink.Log(ctx, protocol.Log{LeaseID: e.lease, Text: l}); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.state(t); got != "running" {
		t.Fatalf("the first sign of life must start the attempt, state %s", got)
	}
	var got []string
	ra.attach(func(b []byte) { got = append(got, string(b)) })
	if err := e.sink.Log(ctx, protocol.Log{LeaseID: e.lease, Text: "three"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "one\n|two\n|three\n" {
		t.Fatalf("lines = %q", got)
	}
	if err := e.sink.Log(ctx, protocol.Log{LeaseID: "lease-unknown", Text: "x"}); err == nil {
		t.Fatal("a log for a lease nobody awaits was accepted")
	}
}

func TestRemoteAwaitResultIsFencedAndMovesToFinalizing(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	ctx := context.Background()
	stale := protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch) + 1, Evidence: evJSON("pass")}
	if err := e.sink.Result(ctx, stale); err == nil {
		t.Fatal("a result for another epoch was accepted")
	}
	if got := e.state(t); got == "finalizing" || got == "passed" {
		t.Fatalf("a stale result moved the attempt to %s", got)
	}
	select {
	case <-ra.result:
		t.Fatal("a stale result reached the run")
	default:
	}
	if err := e.sink.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evJSON("pass")}); err != nil {
		t.Fatal(err)
	}
	if got := e.state(t); got != "finalizing" {
		t.Fatalf("state %s, want finalizing (the run settles it from the evidence)", got)
	}
	if ev := <-ra.result; ev.Result != "pass" {
		t.Fatalf("evidence %+v", ev)
	}
	if err := e.sink.Result(ctx, protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: evJSON("pass")}); err == nil {
		t.Fatal("a second result for the same lease was accepted")
	}
}

func TestRemoteAwaitUnreadableEvidenceIsAnErrorNeverAPass(t *testing.T) {
	other := func(mut func(*model.Evidence)) json.RawMessage {
		var ev model.Evidence
		_ = json.Unmarshal(evJSON("pass"), &ev)
		mut(&ev)
		raw, _ := json.Marshal(ev)
		return raw
	}
	for name, raw := range map[string]json.RawMessage{
		"not json": json.RawMessage(`"x"`), "no result": json.RawMessage(`{"schema":"ci.evidence/v1"}`),
		"wrong schema":   other(func(e *model.Evidence) { e.Schema = "bogus" }),
		"other attempt":  other(func(e *model.Evidence) { e.Attempt = &model.AttemptRef{ID: "another", N: 1} }),
		"no attempt":     other(func(e *model.Evidence) { e.Attempt = nil }),
		"other revision": other(func(e *model.Evidence) { e.Revision = "ffffffffffffffffffffffffffffffffffffffff" }),
		"pass with a failing check": other(func(e *model.Evidence) {
			e.Checks = []model.Check{{ID: "c", JobID: "j", Result: "fail"}}
		}),
	} {
		e := newRemoteEnv(t)
		ra := e.sink.expect(e.id, testRev)
		if err := e.sink.Result(context.Background(), protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: raw}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// an unreadable document is an error result, which is an infra failure: the attempt is lost (D-1103) and never a pass
		select {
		case ev := <-ra.result:
			t.Errorf("%s: result %q delivered, want the attempt lost", name, ev.Result)
		default:
			if got := e.state(t); got != "lost" {
				t.Errorf("%s: state %s, want lost", name, got)
			}
		}
	}
}

func TestRemoteAwaitCarrierEndWithoutResultIsLost(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	cause := errors.New("transport.ssh_session_lost")
	e.sink.ended(e.id, cause)
	e.sink.ended(e.id, errors.New("second report must not block"))
	if got := <-ra.lost; !errors.Is(got, cause) {
		t.Fatalf("lost = %v", got)
	}
}

func TestRemoteAwaitCancelCarrierStopsTheDriverAndSignalsEnd(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	ctx, end := e.sink.carrierStart(context.Background(), e.id, e.lease)
	if err := RenewLease(context.Background(), e.lease, uint64(e.epoch)); err != nil {
		t.Fatalf("a live lease must renew: %v", err)
	}
	e.sink.cancelCarrier(e.id)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the carrier context was not cancelled")
	}
	select {
	case <-ra.ended:
		t.Fatal("ended before the carrier returned")
	default:
	}
	end()
	select {
	case <-ra.ended:
	default:
		t.Fatal("end did not signal the run")
	}
	if err := RenewLease(context.Background(), e.lease, uint64(e.epoch)); err == nil {
		t.Fatal("a lease whose carrier ended still renews")
	}
}

func TestRemoteAwaitRenewRefusesAStaleEpoch(t *testing.T) {
	e := newRemoteEnv(t)
	e.sink.expect(e.id, testRev)
	_, end := e.sink.carrierStart(context.Background(), e.id, e.lease)
	defer end()
	if err := RenewLease(context.Background(), e.lease, uint64(e.epoch)+7); err == nil {
		t.Fatal("renewing a stale epoch succeeded")
	}
}

// registerNode puts an online ssh node, trusted for owner jobs, in the registry of st.
func registerNode(t *testing.T, st *store.Store, id, project string) {
	t.Helper()
	ctx := context.Background()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: project, Platform: platform(), CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	lan, owned, uid := "lan", "owned", true
	c.Identity.ID, c.Identity.Name, c.Identity.Provider, c.Identity.Transport = id, id, "ssh", "ssh"
	c.Identity.SSH = &model.CapabilitySSH{Hostname: "10.0.0.9", Port: 22, User: "ci"}
	c.Location.Kind.Value, c.Economics.Class.Value = &lan, &owned
	c.Separation.UIDSeparation = model.Fact[bool]{Value: &uid, Source: "self", ObservedAt: time.Now(), Confidence: "known"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(ctx, store.NodeRecord{ID: id, Name: id, State: "offline", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "test"); err != nil {
		t.Fatal(err)
	}
	admin, err := store.NewAdminAuthority("test")
	if err != nil {
		t.Fatal(err)
	}
	reg := registry.New(st)
	tr := model.CapabilityTrust{
		Accepts:       assigned([]model.TrustClass{"owner", "internal"}),
		Isolation:     assigned(model.Isolation("container")),
		Network:       assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}),
		SecretClasses: assigned([]model.SecretClass{"none"}),
		PrivacyZone:   assigned(model.PrivacyZone("private-infrastructure")),
	}
	if _, err = reg.SetTrust(ctx, id, tr, admin); err != nil {
		t.Fatal(err)
	}
	if _, err = reg.SetState(ctx, id, "online", "", "test"); err != nil {
		t.Fatal(err)
	}
}

func assigned[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: time.Now(), Confidence: "known"}
}

func testCapacity() ciexec.Capacity {
	return ciexec.Capacity{CPUs: 8, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
}

func registryRun(t *testing.T, clock func() time.Time) (*runDispatch, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	registerNode(t, st, "n1", "/proj")
	registerNode(t, st, "n2", "/proj")
	pol := sched.Policy{Preset: "balanced"}
	sess := &Session{Store: st, RunID: "r1", Capacity: testCapacity, Clock: clock}
	return &runDispatch{sess: sess, row: store.PipelineRow{ID: "r1", Project: "/proj", SourceTrust: "owner"}, pol: &pol}, st
}

func TestHookWorldHasRegistryNodes(t *testing.T) {
	rd, _ := registryRun(t, nil)
	w, _, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]sched.Runner{}
	for _, r := range w.Runners {
		ids[r.ID] = r
	}
	if len(ids) != 3 || ids["local"].ID == "" || ids["n1"].ID == "" || ids["n2"].ID == "" {
		t.Fatalf("the hook's world must hold this machine and every registered node, got %v", w.Runners)
	}
	if !ids["local"].Decl.PersonalOwnNode || ids["n1"].Decl.PersonalOwnNode {
		t.Fatal("only the injected local runner is the PERSONAL own node")
	}
	job := sched.Job{Key: "r1.a.a1", ID: "r1.a", Name: "a", Pipeline: sched.Pipeline{ID: "r1", Support: []string{platform()}},
		Requirements: model.PlacementRequirements{Platform: platform(), CPU: 1, MemMB: 256}, Trust: "owner", Network: "internet", TimeoutMs: 60000,
		RemoteOnly: true, PersonalOwnRun: true, Project: "/proj"}
	got := sched.Place(w, []sched.Job{job})
	if len(got.Placements) != 1 || got.Placements[0].RunnerID == "local" {
		t.Fatalf("a remote-only owner job must be placeable on a registered node: %+v %+v", got.Placements, got.Explanations)
	}
}

func TestHookWorldTreatsARegisteredNodeCalledLocalAsTheLocalRunner(t *testing.T) {
	rd, st := registryRun(t, nil)
	registerNode(t, st, "local", "/proj")
	w, _, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, r := range w.Runners {
		if r.ID == "local" {
			n++
			if r.Decl.PersonalOwnNode {
				t.Fatal("a registered node called local must not inherit the personal-node declaration")
			}
		}
	}
	if n != 1 {
		t.Fatalf("%d runners called local; a registered local node is the local runner, as in `ci plan`", n)
	}
}

// What `ci plan` builds (cmd/plan_placement.go) and what a run builds are the same call.
func TestPlanRunWorldDigestEqual(t *testing.T) {
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	rd, st := registryRun(t, func() time.Time { return fixed })
	runW, runDigest, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	capacity := testCapacity()
	planW, planDigest, err := world.BuildForLocal(context.Background(), world.Deps{Store: st, Clock: func() time.Time { return fixed },
		Jobs:   []sched.Job{{Key: "plan", ID: "plan", Pipeline: sched.Pipeline{ID: "r1"}}},
		Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{Preset: "balanced"}, nil }},
		world.LocalInput{Project: "/proj", Platform: runtime.GOOS + "/" + runtime.GOARCH, Capacity: capacity, DockerUnknown: true, Now: fixed}, sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if runDigest != planDigest {
		a, _ := json.Marshal(runW)
		b, _ := json.Marshal(planW)
		t.Fatalf("plan and run describe the same machine and nodes differently:\n run  %s\n plan %s", a, b)
	}
}

// A run places by the docker and tool facts its plan measured, so the digest of
// a plan that probed this machine equals the run's (D-1104). A plan that
// measured nothing leaves docker unknown, as before.
func TestRunWorldFollowsThePlansMeasuredFacts(t *testing.T) {
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	rd, st := registryRun(t, func() time.Time { return fixed })
	rd.sess.Plan = &plan.Plan{World: plan.World{Measured: true, Docker: true, Tools: []string{"go", "pnpm"}}}
	_, runDigest, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	digestOf := func(in world.LocalInput) string {
		t.Helper()
		in.Project, in.Platform, in.Capacity, in.Now = "/proj", runtime.GOOS+"/"+runtime.GOARCH, testCapacity(), fixed
		_, d, err := world.BuildForLocal(context.Background(), world.Deps{Store: st, Clock: func() time.Time { return fixed },
			Jobs:   []sched.Job{{Key: "plan", ID: "plan", Pipeline: sched.Pipeline{ID: "r1"}}},
			Policy: func(context.Context, string) (sched.Policy, error) { return sched.Policy{Preset: "balanced"}, nil }}, in, sched.Pipeline{ID: "r1"})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	if want := digestOf(world.LocalInput{Docker: true, Tools: []string{"go", "pnpm"}}); runDigest != want {
		t.Fatal("the run's world digest differs from the digest of the plan's measured world")
	}
	if runDigest == digestOf(world.LocalInput{DockerUnknown: true}) {
		t.Fatal("the run still reports docker and tools as unknown")
	}
	rd.sess.Plan = &plan.Plan{} // nothing measured: unknown, the old behaviour
	_, unmeasured, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"})
	if err != nil || unmeasured != digestOf(world.LocalInput{DockerUnknown: true}) {
		t.Fatalf("an unmeasured plan must keep docker unknown: %v", err)
	}
}

func deliveryFor(n model.Capability, job model.Job) dispatch.Delivery {
	return dispatch.Delivery{Node: n, Job: job, AttemptID: "a1", LeaseID: "lease-a1", Token: "t", Epoch: 1, Revision: "0123456789abcdef0123456789abcdef01234567",
		ExpiresAt: time.Now().Add(time.Minute)}
}

// probeDriver records the lease it is given.
type probeDriver struct {
	kind  string
	mu    sync.Mutex
	lease protocol.Lease
}

func (p *probeDriver) Kind() string { return p.kind }
func (p *probeDriver) Run(_ context.Context, _ model.Capability, l protocol.Lease, _ transport.Sink) error {
	p.mu.Lock()
	p.lease = l
	p.mu.Unlock()
	return nil
}

// A lease carries secret references only to a secrets-eligible node, only for a job placed at a class the gate
// accepts, and never a value (D-1373).
func TestLeaseSecretRefsGoOnlyToAnEligibleNodeThatPassesTheGate(t *testing.T) {
	probe := &probeDriver{kind: fmt.Sprintf("secretsprobe%d", time.Now().UnixNano())}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	node := func(ownership string, eligible bool) model.Capability {
		var n model.Capability
		n.Identity.ID, n.Identity.Transport = "x1", probe.kind
		n.Identity.Ownership.Value = &ownership
		n.SecretsEligible = eligible
		return n
	}
	project := model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/KEY"}}
	release := model.Job{Kind: "release", Run: []string{"true"}, Secrets: []string{"release/TOKEN", "project/KEY"}}
	dedicated := trust.RunnerFacts{Ownership: "operator", Dedicated: true, UIDSeparation: true, Isolation: "vm"}
	for _, c := range []struct {
		name     string
		node     model.Capability
		job      model.Job
		trust    model.TrustClass
		runner   trust.RunnerFacts
		wantRefs int
		wantErr  string
	}{
		{"foreign node, not eligible", node("third-party", false), release, "owner", dedicated, 0, ""},
		{"operator node, not eligible", node("operator", false), project, "owner", dedicated, 0, ""},
		{"no secrets declared", node("operator", true), model.Job{Kind: "test", Run: []string{"true"}}, "owner", dedicated, 0, ""},
		{"eligible node, project secret", node("operator", true), project, "owner", trust.RunnerFacts{Ownership: "operator", UIDSeparation: true}, 1, ""},
		{"eligible dedicated node, release secret", node("operator", true), release, "owner", dedicated, 2, ""},
		{"release secret on a node that is not dedicated", node("operator", true), release, "owner", trust.RunnerFacts{Ownership: "operator", UIDSeparation: true, Isolation: "process"}, 0, "E677"},
		{"untrusted source", node("operator", true), project, "untrusted", dedicated, 0, "E677"},
		{"no trust class", node("operator", true), project, "", dedicated, 0, "E677"},
		{"hosted runner", node("operator", true), project, "owner", trust.RunnerFacts{Hosted: true}, 0, "E677"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := deliveryFor(c.node, c.job)
			d.Trust, d.Runner = c.trust, c.runner
			err := newTransportAdapter(nil, nil, "").Deliver(context.Background(), d)
			if c.wantErr != "" {
				var se *secrets.Error
				if !errors.As(err, &se) || se.Code != c.wantErr {
					t.Fatalf("Deliver = %v, want %s", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			probe.mu.Lock()
			got := probe.lease
			probe.mu.Unlock()
			if len(got.Secrets) != c.wantRefs {
				t.Fatalf("the lease carries %d refs %v, want %d", len(got.Secrets), got.Secrets, c.wantRefs)
			}
			if (c.wantRefs > 0) != (got.Trust != "") || c.wantRefs > 0 && got.Trust != c.trust {
				t.Fatalf("lease trust = %q with %d refs", got.Trust, c.wantRefs)
			}
			for _, r := range got.Secrets {
				if r.Class == "" || strings.Contains(r.Ref, "=") {
					t.Fatalf("ref %+v is not a bare reference", r)
				}
			}
		})
	}
}

func TestRemoteAwaitPendingLogIsBounded(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	chunk := make([]byte, 64<<10)
	for i := 0; i < 64; i++ { // 4 MiB offered before the run attaches
		if err := e.sink.Log(context.Background(), protocol.Log{LeaseID: e.lease, Text: string(chunk)}); err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	truncated := false
	ra.attach(func(b []byte) {
		total += len(b)
		truncated = truncated || strings.Contains(string(b), "output truncated")
	})
	if total > maxPendingLog+4096 || !truncated {
		t.Fatalf("held %d bytes, truncated=%v; want at most %d and an explicit marker", total, truncated, maxPendingLog)
	}
}

func TestRemoteAwaitDetachDropsLaterOutput(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id, testRev)
	got := 0
	ra.attach(func(b []byte) { got += len(b) })
	ra.detach()
	_ = e.sink.Log(context.Background(), protocol.Log{LeaseID: e.lease, Text: "late"})
	if got != 0 {
		t.Fatal("output reached a run that had finished")
	}
}

type fakeRedactor struct{ held []byte }

func (f *fakeRedactor) Redact(b []byte) []byte {
	out := strings.ReplaceAll(string(append(f.held, b...)), "hunter2", "[masked]")
	f.held = nil
	if i := strings.LastIndex(out, "hunt"); i >= 0 && i > len(out)-7 { // hold a possible split secret
		f.held = []byte(out[i:])
		out = out[:i]
	}
	return []byte(out)
}
func (f *fakeRedactor) Flush() []byte { b := f.held; f.held = nil; return b }

func TestRemoteAwaitNodeOutputIsRedactedBeforeItIsStored(t *testing.T) {
	var got strings.Builder
	emit, flush := newEmitter(&fakeRedactor{}, func(b []byte) { got.Write(b) })
	emit([]byte("token=hunt"))
	emit([]byte("er2 done\n"))
	flush()
	if strings.Contains(got.String(), "hunter2") || !strings.Contains(got.String(), "[masked]") {
		t.Fatalf("a secret split across two node frames reached the log: %q", got.String())
	}
	var plain strings.Builder
	emit, flush = newEmitter(nil, func(b []byte) { plain.Write(b) })
	emit([]byte("x"))
	flush()
	if plain.String() != "x" {
		t.Fatalf("no redactor must pass output through, got %q", plain.String())
	}
}

func TestRemoteAwaitDriveToCancelledFromEveryLiveState(t *testing.T) {
	for _, path := range [][]string{{}, {"running"}, {"running", "finalizing"}, {"running", "lost"}, {"running", "lost", "retryable"}} {
		e := newRemoteEnv(t)
		for _, to := range path {
			_, epoch, _ := e.st.AttemptStateEpoch(context.Background(), e.id)
			from, _, _ := e.st.AttemptStateEpoch(context.Background(), e.id)
			if err := e.st.Transition(context.Background(), e.id, from, model.JobState(to), store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
				t.Fatalf("%v: %v", path, err)
			}
		}
		rs := &runState{sess: &Session{Store: e.st}}
		if err := rs.driveToCancelled(e.id); err != nil {
			t.Fatalf("%v: %v", path, err)
		}
		if got := e.state(t); got != "cancelled" {
			t.Errorf("from %v the attempt ended %s, want cancelled", path, got)
		}
	}
}

func TestRemoteAwaitNodeFaultOnlyCountsAgainstTheNode(t *testing.T) {
	for _, c := range []struct {
		err  string
		node bool
	}{{"E656 transport.ssh_session_lost: x", true}, {"E656 transport.ssh_connect_failed: x", true}, {"E656 transport.ssh_connect_timeout: x", true},
		{"E655 transport.ssh_host_key_changed: x", true}, {"E656 transport.ssh_checkout: no repo", false},
		{"E656 transport.ssh_not_configured: x", false}, {"E656 transport.ssh_secrets_refused: x", false},
		{"E656 transport.ssh_connect_failed: cannot reach node: transport.ssh_checkout", true}, // text after the reason is irrelevant
		{"E626 database is locked (SQLITE_BUSY)", false}, {"log for unknown lease lease-x", false},
		{"E656 transport.ssh_checkout: transport.ssh_session_lost", false}, // a node cannot talk its way onto the list
	} {
		if got := nodeFault(errors.New(c.err)); got != c.node {
			t.Errorf("%s: nodeFault = %v", c.err, got)
		}
	}
}

func TestRemoteAwaitLocalOnlyJobs(t *testing.T) {
	rs := &runState{c: &Coordinator{}, hooks: Hooks{}}
	for name, c := range map[string]struct {
		job  model.Job
		opts func(*Options)
		want bool
	}{
		"plain":              {model.Job{Kind: "test", Run: []string{"true"}}, nil, false},
		"remote only":        {model.Job{Kind: "test", Run: []string{"true"}, RemoteOnly: true}, nil, false},
		"artifacts":          {model.Job{Kind: "build", Run: []string{"true"}, Artifacts: []model.ArtifactDecl{{Path: "out"}}}, nil, true},
		"cache":              {model.Job{Kind: "build", Run: []string{"true"}, Cache: &model.JobCacheConfig{}}, nil, true},
		"secrets":            {model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/K"}}, nil, true},
		"secrets remote":     {model.Job{Kind: "test", Run: []string{"true"}, Secrets: []string{"project/K"}, RemoteOnly: true}, nil, true},
		"services":           {model.Job{Kind: "test", Run: []string{"true"}, NeedsServices: []string{"db"}}, nil, true},
		"labels":             {model.Job{Kind: "test", Run: []string{"true"}, Requirements: &model.Requirements{Labels: []string{"gpu"}}}, nil, true},
		"labels remote only": {model.Job{Kind: "test", Run: []string{"true"}, Requirements: &model.Requirements{Labels: []string{"gpu"}}, RemoteOnly: true}, nil, false},
		"isolation":          {model.Job{Kind: "test", Run: []string{"true"}, Isolation: "container"}, nil, true},
		"--container":        {model.Job{Kind: "test", Run: []string{"true"}}, func(o *Options) { o.Container = true }, true},
		"--network":          {model.Job{Kind: "test", Run: []string{"true"}}, func(o *Options) { o.Network = "none" }, true},
		"release":            {model.Job{Kind: "release", Run: []string{"true"}}, nil, true},
		"at most once":       {model.Job{Kind: "test", Run: []string{"true"}, Idempotent: new(bool)}, nil, true},
		"planner isolation":  {model.Job{Kind: "test", Run: []string{"true"}, Isolation: "process"}, nil, false},
		"--service":          {model.Job{Kind: "test", Run: []string{"true"}}, func(o *Options) { o.Services = []string{"db"} }, true},
		"--timeout":          {model.Job{Kind: "test", Run: []string{"true"}}, func(o *Options) { o.Timeout = time.Minute }, true},
	} {
		rs.c.Opts = Options{}
		if c.opts != nil {
			c.opts(&rs.c.Opts)
		}
		if got := rs.localOnly(c.job); got != c.want {
			t.Errorf("%s: localOnly = %v, want %v", name, got, c.want)
		}
	}
}

func TestHookWorldKeepsALocalOnlyJobOffNodes(t *testing.T) {
	rd, _ := registryRun(t, nil)
	rd.sess.LocalOnly = func(model.Job) bool { return true }
	job := model.Job{Kind: "test", Run: []string{"true"}}
	sj := rd.schedJob(AttemptSpec{Job: &job, N: 1, Foreground: true}, "r1.a.a1", "r1.a", 1, 256)
	if sj.Pipeline.ID != "r1" {
		t.Fatalf("pipeline id %q: a placement constraint must not ride on the pipeline id (telemetry and approvals key on it)", sj.Pipeline.ID)
	}
	w, _, err := rd.world(context.Background(), sj.Pipeline)
	if err != nil {
		t.Fatal(err)
	}
	has := false
	for _, l := range sj.Requirements.Labels {
		has = has || l == world.LocalLabel
	}
	if !has {
		t.Fatalf("a local-only job must require the label of this machine, requirements %+v", sj.Requirements)
	}
	got := sched.Place(w, []sched.Job{sj})
	if len(got.Placements) != 1 || got.Placements[0].RunnerID != "local" {
		t.Fatalf("a local-only job must be placed on this machine: %+v %+v", got.Placements, got.Explanations)
	}
	for _, x := range got.Explanations {
		for _, a := range x.Alternatives {
			if a.RunnerID != "local" && a.Eligible {
				t.Fatalf("a local-only job is eligible on %s", a.RunnerID)
			}
		}
	}
	rd.sess.LocalOnly = nil
	free := rd.schedJob(AttemptSpec{Job: &model.Job{Kind: "test", Run: []string{"true"}, RemoteOnly: true}, N: 1, Foreground: true}, "r1.b.a1", "r1.b", 1, 256)
	for _, l := range free.Requirements.Labels {
		if l == world.LocalLabel {
			t.Fatal("a job nothing holds back was kept off the nodes")
		}
	}
	// the same two jobs against a world: the ordinary one may win a node when it is the better place, the local-only one never does
	if got := sched.Place(w, []sched.Job{free}); len(got.Placements) != 1 || got.Placements[0].RunnerID == "local" {
		t.Fatalf("a remote-only job must go to a node: %+v", got.Placements)
	}
}

type wantsAll struct{}

func (wantsAll) Wants(model.Job) bool { return true }
func (wantsAll) Outcome(context.Context, OutcomeFacts) (OutcomeChange, bool) {
	return OutcomeChange{}, false
}

func TestRemoteAwaitJobsAnOutcomeHookReadsStayLocal(t *testing.T) {
	rs := &runState{c: &Coordinator{}, hooks: Hooks{Outcomes: []OutcomeHook{wantsAll{}}}}
	if !rs.localOnly(model.Job{Kind: "drill", Run: []string{"true"}}) {
		t.Fatal("a job an outcome hook judges by its output ran on a node, where the output never reaches the hook")
	}
}

func TestRemoteAwaitCancelNeedsNoRetryAuthorization(t *testing.T) {
	ctx := context.Background()
	for name, kind := range map[string]string{"release job": "release", "ordinary": "test"} {
		st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
		if err != nil {
			t.Fatal(err)
		}
		if err = st.CreatePipeline(ctx, store.PipelineRow{ID: "p", Project: "/p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
			t.Fatal(err)
		}
		// infra_max 0 and, for the release job, at-most-once: lost -> retryable is forbidden for both
		if err = st.CreateJob(ctx, store.JobRow{ID: "j", PipelineID: "p", Name: "j", Kind: kind, Idempotent: kind != "release", InfraMax: 0, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err = st.CreateAttempt(ctx, store.AttemptRow{ID: "j.a1", JobID: "j", N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		storetest.Lease(t, st, "j.a1")
		_, epoch, _ := st.AttemptStateEpoch(ctx, "j.a1")
		if err = st.Transition(ctx, "j.a1", "leased", "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
			t.Fatal(err)
		}
		rs := &runState{sess: &Session{Store: st}}
		if err = rs.driveToCancelled("j.a1"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got, _, _ := st.AttemptStateEpoch(ctx, "j.a1"); got != "cancelled" {
			t.Errorf("%s: a cancelled run left its lost attempt %s", name, got)
		}
		_ = st.Close()
	}
}

// remoteLost must end in the state the local path uses whichever of the loss, the cancel and a cancel
// intent got there first.
func lostEnv(t *testing.T) (*remoteEnv, *runState, *runDispatch) {
	e := newRemoteEnv(t)
	e.sink.expect(e.id, testRev)
	sess := &Session{Store: e.st, RunID: "run1"}
	rs := &runState{c: &Coordinator{}, sess: sess, marks: map[string]model.Reason{}, cancels: map[string]context.CancelFunc{}, attempts: map[string]string{}}
	return e, rs, &runDispatch{sess: sess, sink: e.sink}
}

func planJob(id string) plan.PlannedJob { return plan.PlannedJob{ID: id, Name: id} }

func TestRemoteAwaitLossRacingTheRunCancelEndsCancelled(t *testing.T) {
	e, rs, rd := lostEnv(t)
	_, epoch, _ := e.st.AttemptStateEpoch(context.Background(), e.id)
	if err := e.st.Transition(context.Background(), e.id, "leased", "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil { // the dispatcher marked it lost
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // and Ctrl-C reached the run too
	o := rs.remoteLost(ctx, planJob("unit"), model.Job{}, rd, e.id, errors.New("E656 transport.ssh_session_lost"), JobOutcome{ID: "unit"}, 1, &tail{})
	if o.State != StateCancelled || e.state(t) != "cancelled" {
		t.Fatalf("outcome %q, durable %s; a cancelled run must end its attempt cancelled", o.State, e.state(t))
	}
}

func TestRemoteAwaitCancelIntentBeatsTheLossSettlement(t *testing.T) {
	e, rs, rd := lostEnv(t)
	_, epoch, _ := e.st.AttemptStateEpoch(context.Background(), e.id)
	if err := e.st.Transition(context.Background(), e.id, "leased", "running", store.TransitionDetail{Epoch: epoch}); err != nil {
		t.Fatal(err)
	}
	_, epoch, _ = e.st.AttemptStateEpoch(context.Background(), e.id)
	if err := e.st.RequestCancel(context.Background(), e.id, "operator", epoch); err != nil { // another process cancelled; the dispatcher's move to lost lost the race (E605)
		t.Fatal(err)
	}
	o := rs.remoteLost(context.Background(), planJob("unit"), model.Job{}, rd, e.id, errors.New("E656 transport.ssh_session_lost"), JobOutcome{ID: "unit"}, 1, &tail{})
	if o.State != StateCancelled || e.state(t) != "cancelled" {
		t.Fatalf("outcome %q, durable %s; the operator's cancel must win and no attempt may stay running", o.State, e.state(t))
	}
}

func TestRemoteAwaitLostThenCancelIntentEndsCancelled(t *testing.T) {
	for name, live := range map[string]bool{"run cancelled": false, "run live": true} {
		e, rs, rd := lostEnv(t)
		_, epoch, _ := e.st.AttemptStateEpoch(context.Background(), e.id)
		if err := e.st.Transition(context.Background(), e.id, "leased", "lost", store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
			t.Fatal(err)
		}
		_, epoch, _ = e.st.AttemptStateEpoch(context.Background(), e.id)
		if err := e.st.RequestCancel(context.Background(), e.id, "operator", epoch); err != nil { // `ci runs cancel` after the dispatcher marked it lost
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if !live {
			cancel()
		}
		job := model.Job{Retry: &model.RetryConfig{InfraMax: 1}} // a retry is on offer: the intent must still win
		o := rs.remoteLost(ctx, planJob("unit"), job, rd, e.id, errors.New("E656 transport.ssh_session_lost"), JobOutcome{ID: "unit"}, 1, &tail{})
		cancel()
		if e.state(t) != "cancelled" {
			t.Errorf("%s: durable %s (outcome %q), want cancelled: a lost attempt with a cancel intent must not be stuck", name, e.state(t), o.State)
		}
	}
}

func TestRemoteAwaitCancelFromRetryableNeedsNoQueueMove(t *testing.T) {
	e, rs, _ := lostEnv(t)
	ctx := context.Background()
	for _, to := range []string{"lost", "retryable"} {
		from, epoch, _ := e.st.AttemptStateEpoch(ctx, e.id)
		if err := e.st.Transition(ctx, e.id, from, model.JobState(to), store.TransitionDetail{Epoch: epoch, FailureClass: "infra"}); err != nil {
			t.Fatal(err)
		}
	}
	_, epoch, _ := e.st.AttemptStateEpoch(ctx, e.id)
	if err := e.st.RequestCancel(ctx, e.id, "operator", epoch); err != nil { // the intent lands between park's two transactions
		t.Fatal(err)
	}
	if err := rs.driveToCancelled(e.id); err != nil || e.state(t) != "cancelled" {
		t.Fatalf("err %v, durable %s; a retryable attempt with a cancel intent must still end cancelled", err, e.state(t))
	}
}

// Nothing but the injected local runner carries the label that lets a job stay on this machine.
func TestHookWorldOnlyTheInjectedRunnerCarriesTheLocalLabel(t *testing.T) {
	rd, st := registryRun(t, nil)
	// a node an operator (or a bug) labelled as this machine, and a node called local
	reg := registry.New(st)
	n, err := reg.Get(context.Background(), "n1")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = reg.UpdateCapability(context.Background(), "n1", withLabel(n.Capability, world.LocalLabel), "test"); err != nil {
		t.Skipf("labels cannot be set through UpdateCapability here: %v", err)
	}
	w, _, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range w.Runners {
		has := false
		for _, l := range r.Capability.Identity.Labels {
			has = has || l == world.LocalLabel
		}
		if has != (r.ID == "local") {
			t.Errorf("runner %s: carries the local label = %v", r.ID, has)
		}
	}
	job := sched.Job{Key: "r1.a.a1", ID: "r1.a", Name: "a", Pipeline: sched.Pipeline{ID: "r1", Support: []string{platform()}},
		Requirements: model.PlacementRequirements{Platform: platform(), CPU: 1, MemMB: 256, Labels: []string{world.LocalLabel}}, Trust: "owner", Network: "internet",
		TimeoutMs: 60000, PersonalOwnRun: true, Project: "/proj"}
	if got := sched.Place(w, []sched.Job{job}); len(got.Placements) != 1 || got.Placements[0].RunnerID != "local" {
		t.Fatalf("a local-only job must reach only this machine: %+v", got.Placements)
	}
}

func withLabel(c model.Capability, l string) model.Capability {
	c.Identity.Labels = append(append([]string(nil), c.Identity.Labels...), l)
	return c
}

// A cancelled run stops the carrier: the carrier's context is cancelled before the run gives up waiting.
func TestRemoteAwaitCancelStopsTheCarrierBeforeTheRunGivesUp(t *testing.T) {
	e, rs, rd := lostEnv(t)
	defer SetRemoteCancelWait(300 * time.Millisecond)()
	cctx, end := e.sink.carrierStart(context.Background(), e.id, e.lease)
	var stopped atomic.Bool
	go func() { // the driver: it returns when its context is cancelled
		select {
		case <-cctx.Done():
			stopped.Store(true)
			end()
		case <-time.After(5 * time.Second):
		}
	}()
	ra := e.sink.attempt(e.id)
	o := rs.remoteCancelled(planJob("unit"), rd, ra, e.id, JobOutcome{ID: "unit"})
	if !stopped.Load() || o.State != StateCancelled {
		t.Fatalf("carrier stopped = %v, outcome %q: the run gave up before the carrier was told to stop", stopped.Load(), o.State)
	}
	if e.state(t) != "cancelled" {
		t.Fatalf("durable state %s", e.state(t))
	}
}
