//go:build integration

package runnerjob_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/protocol"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/demand"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/transport"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const simSHA = "cccccccccccccccccccccccccccccccccccccccc"

func labelsFact(labels ...string) model.Fact[[]string] {
	return model.Fact[[]string]{Value: &labels, Source: "payload", ObservedAt: epoch, Confidence: "known"}
}

func strFact(s string) model.Fact[string] {
	return model.Fact[string]{Value: &s, Source: "payload", ObservedAt: epoch, Confidence: "known"}
}

// queueAttempt writes the pipeline, job and queued attempt rows of the synthetic pipeline.
func queueAttempt(t *testing.T, st *store.Store, p *model.Pipeline, attempt string) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreatePipeline(ctx, store.PipelineRow{ID: p.ID, Revision: simSHA, Project: testRepo, Trigger: "api", SourceTrust: "untrusted", PrivacyZone: "private-infrastructure",
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

// TestJITSim proves acceptance lines 2 and 3 on the sim: a queued workflow_job becomes a pipeline through the
// registered GitHub demand handler; the demand is admitted through dispatch.Admit BEFORE anything is
// registered with the forge; delivery runs the preparer, which mints one JIT config with only the fixed
// labels; the stub runner reads it from the credential wrapper; when the run closes the runner is
// deregistered and absent from the forge's list; an orphan older than 70 minutes is deleted by the sweep
// and a persistent runner of the repository is left alone.
func TestJITSim(t *testing.T) {
	needDocker(t, stubImage)
	st := use(t)
	ctx := context.Background()
	gh := newFakeGH(t)
	reg := gh.registrar(t, jit.DefaultSettings(), clock)
	demand.UseRegistrar(func(context.Context) (*jit.Registrar, error) { return reg, nil })
	t.Cleanup(func() { demand.UseRegistrar(nil) })

	// 1. the demand: labels [self-hosted, nself] -> one synthetic pipeline
	h := providers.DemandHandlerFor("github")
	if h == nil {
		t.Fatal("the github demand handler is not registered")
	}
	facts := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: "github", Kind: "runner_demand", DeliveryID: "d-1", ReceivedAt: epoch, Auth: "per-repo", Repo: testRepo,
		Revision: simSHA, RevisionSource: "payload", RunnerLabels: labelsFact("self-hosted", "nself", "linux", "x64"), ProviderJobID: strFact("4001"), ProviderRunID: strFact("3001")}
	p, err := h.Synthesize(ctx, facts)
	if err != nil || p == nil {
		t.Fatalf("synthesize: %v", err)
	}
	raw, _ := json.Marshal(p) // what the intake stores
	var stored model.Pipeline
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	runnerjob.Restore(&stored, runnerjob.KindOf)
	job := stored.Jobs[0]
	if job.RunnerCredential != "github-jitconfig" || !job.RemoteOnly || runnerjob.BandOf(&stored) != "background" {
		t.Fatalf("restored job %+v", job)
	}

	// 2. admission: dispatch.Admit commits the lease; nothing is registered yet
	queueAttempt(t, st, &stored, "sim-attempt")
	node := goodNode()
	node.Identity.Provider = "operator"
	sj := planview.SchedJob(planview.Context{PipelineID: stored.ID, Project: testRepo, Support: []string{"linux/amd64"}, DefaultPlatform: "linux/amd64"}, planview.PlannedJob{ID: "runner", Name: "runner", Job: job})
	runnerjob.Constrain(&sj)
	sj.Key, sj.Attempt = "sim-attempt", 1
	runner := sched.Runner{ID: "node-1", Location: "lan", Class: "owned", Provider: "operator", Capability: node, Avail: sched.Avail{Slots: 1, CPU: 8, MemMB: 8192},
		Decl: trust.Declaration{Accepts: []model.TrustClass{"untrusted"}, Isolation: "sandboxed-container", Network: "restricted", UIDSeparation: true, Ownership: "operator", NoLANRoute: true}}
	adm, err := dispatch.Admit(ctx, st, dispatch.Request{AttemptID: "sim-attempt", Placement: sched.Placement{RunnerID: "node-1", CPU: 1, MemMB: 512}, Runner: runner, Job: sj,
		Host: "node-1", CapacityCPU: 8, CapacityMemMB: 8192, RunnerSlots: 1, SlotsKnown: true, LeaseTTL: time.Hour})
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if len(gh.minted) != 0 || gh.count() != 0 {
		t.Fatalf("a registration exists before delivery: minted %d", len(gh.minted))
	}

	// 3. delivery: the preparer mints after admission and fills lease.runner_credential
	lease := protocol.Lease{LeaseID: adm.LeaseID, AttemptID: adm.AttemptID, Epoch: uint64(adm.Epoch), Token: adm.Token, CompatMode: protocol.Compat15, Job: job,
		Checkout: protocol.Checkout{Revision: simSHA}, ExpiresAt: epoch.Add(time.Hour).Format(time.RFC3339)}
	if err = transport.Deliver(ctx, node, lease, nil); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	driver.mu.Lock()
	got := driver.lease
	driver.mu.Unlock()
	if len(gh.minted) != 1 || got.RunnerCredential == nil || got.RunnerCredential.Kind != "github-jitconfig" || got.RunnerCredential.Value != "synthetic-jitconfig-101" {
		t.Fatalf("minted %d, lease credential %+v", len(gh.minted), got.RunnerCredential)
	}
	body := gh.minted[0]
	labels := body["labels"].([]any)
	if len(labels) != 5 || labels[0] != "self-hosted" || labels[1] != "nself" || labels[2] != "linux" || labels[3] != "X64" || labels[4] != body["name"] ||
		!strings.HasPrefix(body["name"].(string), "nself-jit-") {
		t.Fatalf("registration body %v", body)
	}
	if exp, perr := time.Parse(time.RFC3339, got.RunnerCredential.ExpiresAt); perr != nil || exp.Sub(clock()) > time.Hour {
		t.Fatalf("expiry %q must be RFC 3339 and within 60 minutes", got.RunnerCredential.ExpiresAt)
	}

	// 4. the stub runner reads the value from the wrapper; the forge still lists the runner meanwhile
	run, ran := runLeg(t, "host19-sim", stubImage, got.RunnerCredential.Kind, got.RunnerCredential.Value, []string{"sh", "-c", stubScript}, false, 90*time.Second)
	if !ran || run.Res.ExitCode != 0 || !strings.Contains(run.Out, "visible-in-env=1") || !strings.Contains(run.Out, "len="+itoa(len(got.RunnerCredential.Value))) {
		t.Fatalf("stub run: ran=%v %+v\n%s", ran, run.Res, run.Out)
	}
	if gh.count() != 1 {
		t.Fatalf("the runner should still be registered until the run closes, forge lists %v", gh.names())
	}

	// 5. the run closes: deregistered and absent
	if err = runnerjob.Finalize(ctx, []string{"sim-attempt"}); err != nil {
		t.Fatal(err)
	}
	if gh.count() != 0 {
		t.Fatalf("the runner must be absent from the forge list, still there: %v", gh.names())
	}

	// 6. the orphan sweep: a registration born three hours ago and never claimed, beside a persistent runner
	old := gh.registrar(t, jit.DefaultSettings(), func() time.Time { return clock().Add(-3 * time.Hour) })
	if _, err = old.MintRunner(ctx, providers.RunnerRequest{Scope: providers.RunnerScope{Repo: testRepo}, Attempt: providers.Attempt{Trust: "untrusted"}, Labels: []string{"x64"}}); err != nil {
		t.Fatal(err)
	}
	gh.runners = append(gh.runners, fakeRunner{ID: 777, Name: "persistent-runner"})
	if gh.count() != 2 {
		t.Fatalf("setup: %v", gh.names())
	}
	if err = (&runnerjob.Sweeper{Now: clock}).Reconcile(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if names := gh.names(); len(names) != 1 || names[0] != "persistent-runner" {
		t.Fatalf("the sweep must delete the old orphan and only it, forge lists %v", names)
	}
}
