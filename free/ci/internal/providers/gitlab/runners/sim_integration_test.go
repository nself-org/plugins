//go:build integration

package runners_test

// Purpose: the P7-HOST-07 simulation (acceptance 2) and the shared sim fixtures.
// Inputs:  the fake GitLab, a real store behind the runnerjob ledger, a capturing transport driver, the
//          container leg of harness_integration_test.go (a pinned busybox as the stub runner image).
// Outputs: pass or fail.
// Constraints: Docker is required (a missing daemon fails); no live GitLab call; every credential synthetic.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/runners"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const testDriver = "runners-test"

// capture is a transport driver that keeps the lease it was handed.
type capture struct {
	mu    sync.Mutex
	lease protocol.Lease
}

func (*capture) Kind() string { return testDriver }
func (c *capture) Run(_ context.Context, _ model.Capability, l protocol.Lease, _ transport.Sink) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lease = l
	return nil
}

var (
	driver    = &capture{}
	driverReg sync.Once
)

// useSim binds the store, the ledger, V15 mode and the transport driver for one test.
func useSim(t *testing.T) *store.Store {
	t.Helper()
	st, _ := useSimAt(t)
	return st
}

// useSimAt is useSim that also returns the directory of the store.
func useSimAt(t *testing.T) (*store.Store, string) {
	t.Helper()
	driverReg.Do(func() {
		if err := transport.Register(driver); err != nil {
			t.Fatal(err)
		}
	})
	return bindAt(t)
}

func known[T any](v T) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", Confidence: "known", ObservedAt: epoch}
}

// goodNode is a Linux node with gVisor and the restricted scope.
func goodNode() model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity.ID, c.Identity.Transport, c.Identity.Provider = "node-1", testDriver, "operator"
	c.Platform.OS = known("linux")
	c.Trust.Isolation = known(model.Isolation("sandboxed-container"))
	c.Trust.Network = known([]model.NetworkScope{"none", "restricted"})
	c.Tools.GVisor = known(true)
	c.Availability.PluggedIn, c.Availability.Interactive = known(true), known("idle")
	return c
}

// queueAttempt writes the pipeline, job and queued attempt rows of the synthetic pipeline.
func queueAttempt(t *testing.T, st *store.Store, p *model.Pipeline, attempt string) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: p.ID, Revision: simSHA, Project: testProject, Trigger: "api", SourceTrust: "untrusted", PrivacyZone: "private-infrastructure",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-" + attempt, PipelineID: p.ID, Name: "runner", Kind: "build", Idempotent: false, InfraMax: 0, InputDigest: "i", Required: false}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: attempt, JobID: "job-" + attempt, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
}

// admitAndDeliver runs the demand through the registered handler, the intake store round trip, admission and
// delivery, and returns the lease the node received.
func admitAndDeliver(t *testing.T, st *store.Store, attempt, job string, tags ...string) (protocol.Lease, model.Job) {
	t.Helper()
	ctx := context.Background()
	h := providers.DemandHandlerFor("gitlab")
	if h == nil {
		t.Fatal("the gitlab demand handler is not registered")
	}
	p, err := h.Synthesize(ctx, demandFor(job, tags...))
	if err != nil || p == nil {
		t.Fatalf("synthesize: %v", err)
	}
	raw, _ := json.Marshal(p) // what the intake stores
	var stored model.Pipeline
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	runnerjob.Restore(&stored, runnerjob.KindOf)
	j := stored.Jobs[0]
	if j.RunnerCredential != credKind || !j.RemoteOnly || runnerjob.BandOf(&stored) != "background" {
		t.Fatalf("restored job %+v", j)
	}
	queueAttempt(t, st, &stored, attempt)
	node := goodNode()
	sj := planview.SchedJob(planview.Context{PipelineID: stored.ID, Project: testProject, Support: []string{"linux/amd64"}, DefaultPlatform: "linux/amd64"}, planview.PlannedJob{ID: "runner", Name: "runner", Job: j})
	runnerjob.Constrain(&sj)
	sj.Key, sj.Attempt = attempt, 1
	runner := sched.Runner{ID: "node-1", Location: "lan", Class: "owned", Provider: "operator", Capability: node, Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192},
		Decl: trust.Declaration{Accepts: []model.TrustClass{"untrusted"}, Isolation: "sandboxed-container", Network: "restricted", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}}
	adm, err := dispatch.Admit(ctx, st, dispatch.Request{AttemptID: attempt, Placement: sched.Placement{RunnerID: "node-1", CPU: 1, MemMB: 512}, Runner: runner, Job: sj,
		Host: "node-1", CapacityCPU: 8, CapacityMemMB: 8192, RunnerSlots: 1, SlotsKnown: true, LeaseTTL: time.Hour})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if adm.AttemptID != attempt {
		t.Fatalf("admission %+v", adm)
	}
	return protocol.Lease{LeaseID: adm.LeaseID, AttemptID: adm.AttemptID, Epoch: uint64(adm.Epoch), Token: adm.Token, CompatMode: protocol.Compat15, Job: j,
		Checkout: protocol.Checkout{Revision: simSHA}, ExpiresAt: epoch.Add(time.Hour).Format(time.RFC3339)}, j
}

func deliverLease(t *testing.T, node model.Capability, l protocol.Lease) protocol.Lease {
	t.Helper()
	if err := transport.Deliver(context.Background(), node, l, nil); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	driver.mu.Lock()
	defer driver.mu.Unlock()
	return driver.lease
}

const stubScript = `echo "visible-in-env=$(env | grep -c '^CI_SERVER_TOKEN=')"; echo "len=${#CI_SERVER_TOKEN}"; echo "echoed ${CI_SERVER_TOKEN}"; sleep 3`

// TestGitLabRunnerSim proves acceptance 2 on the sim: a pending Job Hook demand with the tag nself becomes a
// pipeline through the registered gitlab handler; the demand is admitted on a gVisor fixture node BEFORE any
// runner exists; delivery mints one runner whose tags are the fixed set; the stub receives the token through
// the wrapper; the fixture records PUT paused=true when the running hook arrives; the run's close deletes the
// runner and it is absent; an orphan older than 70 minutes is deleted by the sweep, a busy old runner and a
// persistent runner are left alone; a demand with an extra tag is E708.
func TestGitLabRunnerSim(t *testing.T) {
	needDocker(t, stubImage)
	st := useSim(t)
	ctx := context.Background()
	f := newFakeGL(t)
	reg := f.registrar(clock)
	runners.UseRegistrar(func(context.Context) (*runners.Registrar, error) { return reg, nil })
	t.Cleanup(func() { runners.UseRegistrar(nil) })

	// a demand with an extra tag never becomes a pipeline
	if _, err := providers.DemandHandlerFor("gitlab").Synthesize(ctx, demandFor("4000", "nself", "ubuntu")); code(err) != "E708" {
		t.Fatalf("an extra tag must be E708, got %v", err)
	}

	lease, job := admitAndDeliver(t, st, "sim-attempt", "4001", "nself")
	if f.count() != 0 || len(f.mints) != 0 {
		t.Fatalf("a runner exists before delivery: %d", f.count())
	}
	got := deliverLease(t, goodNode(), lease)
	if len(f.mints) != 1 || got.RunnerCredential == nil || got.RunnerCredential.Kind != credKind || got.RunnerCredential.Value != "glrt-synthetic-101" {
		t.Fatalf("minted %d, lease credential %+v", len(f.mints), got.RunnerCredential)
	}
	body := f.mints[0]
	tags := body["tag_list"].([]any)
	if len(tags) != 2 || tags[0] != "nself" || tags[1] != body["description"] || !strings.HasPrefix(tags[1].(string), "nself-glr-") ||
		body["run_untagged"] != false || body["locked"] != true || body["access_level"] != "not_protected" || body["project_id"] != float64(testProjectID) {
		t.Fatalf("registration body %v", body)
	}
	if exp, err := time.Parse(time.RFC3339, got.RunnerCredential.ExpiresAt); err != nil || exp.Sub(epoch) > runners.CredentialLifetime {
		t.Fatalf("expiry %q", got.RunnerCredential.ExpiresAt)
	}
	if len(job.Run) == 0 || strings.Contains(strings.Join(job.Run, " "), got.RunnerCredential.Value) {
		t.Fatalf("command %v", job.Run)
	}

	// the stub runner reads the token from the wrapper; meanwhile the running hook arrives
	f.jobs[4001] = fakeJob{Ref: "main", Status: "running", RunnerID: 101}
	hooked := make(chan error, 1)
	go func() {
		time.Sleep(1500 * time.Millisecond)
		ev, ok := runners.ParseJobHook([]byte(sprintf(runningHook, "101")))
		if !ok {
			hooked <- context.Canceled
			return
		}
		_, err := reg.OnRunning(ctx, ev)
		hooked <- err
	}()
	run, ran := runLeg(t, legOptions{ID: "host07-sim", Image: stubImage, Cred: got.RunnerCredential.Value, Command: []string{"sh", "-c", stubScript}, Limit: 90 * time.Second})
	if !ran || run.Res.ExitCode != 0 || !strings.Contains(run.Out, "visible-in-env=1") || !strings.Contains(run.Out, "len="+itoa(int64(len(got.RunnerCredential.Value)))) {
		t.Fatalf("stub run: ran=%v %+v\n%s", ran, run.Res, run.Out)
	}
	if err := <-hooked; err != nil {
		t.Fatalf("running hook: %v", err)
	}
	f.mu.Lock()
	paused, puts := f.runners[0].Paused, len(f.puts)
	f.mu.Unlock()
	if !paused || puts != 1 || f.puts[0]["paused"] != true || f.count() != 1 {
		t.Fatalf("the runner must be paused (PUT paused=true) and still registered: paused %v puts %v count %d", paused, f.puts, f.count())
	}

	// the run closes: deleted and absent
	if err := runnerjob.Finalize(ctx, []string{"sim-attempt"}); err != nil {
		t.Fatal(err)
	}
	if f.count() != 0 {
		t.Fatalf("the runner must be absent from the forge, still there: %v", f.descriptions())
	}

	// the orphan sweep: an idle runner born three hours ago, a busy old one, and a persistent runner
	old := f.registrar(func() time.Time { return epoch.Add(-3 * time.Hour) })
	idle, err := old.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	busy, _ := old.MintRunner(ctx, runnerReq(), runners.MintOptions{})
	f.runners[len(f.runners)-1].Running = true
	f.add("persistent-runner", false)
	if f.count() != 3 {
		t.Fatalf("setup: %v", f.descriptions())
	}
	if err = (&runnerjob.Sweeper{Now: clock}).Reconcile(ctx, nil); err != nil {
		t.Fatal(err)
	}
	left := strings.Join(f.descriptions(), ",")
	if f.count() != 2 || strings.Contains(left, idle.Description) || !strings.Contains(left, busy.Description) || !strings.Contains(left, "persistent-runner") {
		t.Fatalf("the sweep must delete the idle orphan only, forge lists %v", f.descriptions())
	}
}
