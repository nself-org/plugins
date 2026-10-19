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
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
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

func evJSON(result string) json.RawMessage {
	raw, _ := json.Marshal(model.Evidence{Schema: "ci.evidence/v1", Result: model.Result(result)})
	return raw
}

func TestRemoteAwaitLogsAreBufferedUntilTheRunAttaches(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id)
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
	ra := e.sink.expect(e.id)
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
	for name, raw := range map[string]json.RawMessage{"not json": json.RawMessage(`"x"`), "no result": json.RawMessage(`{"schema":"ci.evidence/v1"}`)} {
		e := newRemoteEnv(t)
		ra := e.sink.expect(e.id)
		if err := e.sink.Result(context.Background(), protocol.Result{LeaseID: e.lease, Epoch: uint64(e.epoch), Evidence: raw}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if ev := <-ra.result; ev.Result != "error" {
			t.Errorf("%s: result %q, want error", name, ev.Result)
		}
	}
}

func TestRemoteAwaitCarrierEndWithoutResultIsLost(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id)
	cause := errors.New("transport.ssh_session_lost")
	e.sink.ended(e.id, cause)
	e.sink.ended(e.id, errors.New("second report must not block"))
	if got := <-ra.lost; !errors.Is(got, cause) {
		t.Fatalf("lost = %v", got)
	}
}

func TestRemoteAwaitCancelCarrierStopsTheDriverAndSignalsEnd(t *testing.T) {
	e := newRemoteEnv(t)
	ra := e.sink.expect(e.id)
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
	e.sink.expect(e.id)
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

func TestHookWorldRefusesARegisteredNodeCalledLocal(t *testing.T) {
	rd, st := registryRun(t, nil)
	registerNode(t, st, "local", "/proj")
	if _, _, err := rd.world(context.Background(), sched.Pipeline{ID: "r1"}); err == nil {
		t.Fatal("a registered node called local would inherit the personal-node declaration")
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

func deliveryFor(n model.Capability, job model.Job) dispatch.Delivery {
	return dispatch.Delivery{Node: n, Job: job, AttemptID: "a1", LeaseID: "lease-a1", Token: "t", Epoch: 1, Revision: "0123456789abcdef0123456789abcdef01234567",
		ExpiresAt: time.Now().Add(time.Minute)}
}

// probeDriver records the lease it is given.
type probeDriver struct {
	mu    sync.Mutex
	lease protocol.Lease
}

func (*probeDriver) Kind() string { return "secretsprobe" }
func (p *probeDriver) Run(_ context.Context, _ model.Capability, l protocol.Lease, _ transport.Sink) error {
	p.mu.Lock()
	p.lease = l
	p.mu.Unlock()
	return nil
}

func TestSecretsNeverToForeignNode(t *testing.T) {
	probe := &probeDriver{}
	if err := transport.Register(probe); err != nil {
		t.Fatal(err)
	}
	job := model.Job{Kind: "release", Run: []string{"true"}, Secrets: []string{"release/TOKEN", "project/KEY"}}
	for name, ownership := range map[string]string{"foreign node": "third-party", "operator node": "operator"} {
		var n model.Capability
		n.Identity.ID, n.Identity.Transport = "x1", "secretsprobe"
		n.Identity.Ownership.Value = &ownership
		n.SecretsEligible = ownership == "operator"
		a := newTransportAdapter(nil, nil, "")
		err := a.Deliver(context.Background(), deliveryFor(n, job))
		if err != nil {
			t.Fatal(err)
		}
		probe.mu.Lock()
		got := probe.lease
		probe.mu.Unlock()
		if len(got.Secrets) != 0 {
			t.Errorf("%s: the lease carries secret refs %v; a foreground run sends no secret value to any node", name, got.Secrets)
		}
	}
}
