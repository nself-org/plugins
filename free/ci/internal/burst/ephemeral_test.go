package burst_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var seq atomic.Int64

// queue creates a pipeline (once), a job and its queued attempt.
func queue(t testing.TB, st *store.Store, pipe string) string {
	t.Helper()
	ctx := context.Background()
	_ = st.CreatePipeline(ctx, store.PipelineRow{ID: pipe, Revision: "rev", Project: "project", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "cfg1", SelectionMode: "full", Status: "running"})
	id := pipe + ".job" + strconv.FormatInt(seq.Add(1), 10)
	if err := st.CreateJob(ctx, store.JobRow{ID: "job-" + id, PipelineID: pipe, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i", Required: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "job-" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	return id
}

func admitReq(attempt, runner string, trust model.TrustClass, ephemeral bool, labels ...string) dispatch.Request {
	r := sched.Runner{ID: runner, Provider: "fakecloud", Ephemeral: ephemeral}
	r.Capability.Identity.Labels = labels
	return dispatch.Request{AttemptID: attempt, Placement: sched.Placement{JobKey: attempt, RunnerID: runner, CPU: 1, MemMB: 256}, Runner: r,
		Job: sched.Job{Key: attempt, Trust: trust, TimeoutMs: 60_000}, Local: true, Host: "host-" + attempt, CapacityCPU: 8, CapacityMemMB: 8192}
}

func refusal(t testing.TB, err error) *dispatch.Refusal {
	t.Helper()
	var ref *dispatch.Refusal
	if !errors.As(err, &ref) {
		t.Fatalf("want a refusal, got %v", err)
	}
	return ref
}

// TestEphemeralOneJob: an ephemeral node takes exactly one job, whatever the trust
// class of the first or the second; after the job the intent is torn down and the
// node revoked with the deny-list hook.
func TestEphemeralOneJob(t *testing.T) {
	ctx := context.Background()
	for i, first := range []model.TrustClass{"owner", "collaborator", "internal", "untrusted"} {
		for _, second := range []model.TrustClass{"owner", "collaborator", "internal", "untrusted"} {
			t.Run(string(first)+"_then_"+string(second), func(t *testing.T) {
				r := newRig(t)
				res := r.provision("job-" + strconv.Itoa(i))
				node := "node-e" + strconv.Itoa(i)
				r.join(res.Token, node)
				a1, a2 := queue(t, r.st, "p1"), queue(t, r.st, "p1")
				if _, err := dispatch.Admit(ctx, r.st, admitReq(a1, node, first, true)); err != nil {
					t.Fatalf("the first job: %v", err)
				}
				_, err := dispatch.Admit(ctx, r.st, admitReq(a2, node, second, true))
				if ref := refusal(t, err); ref.Reason != model.EphemeralUsed {
					t.Fatalf("second lease request: %s (%s), want ephemeral.used", ref.Reason, ref.Detail)
				}
				if state, _, _ := r.st.AttemptStateEpoch(ctx, a2); state != "queued" {
					t.Fatalf("the refused attempt must stay queued: %s", state)
				}
			})
		}
	}
}

func TestEphemeralTeardownAfterTheOneJob(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	a1 := queue(t, r.st, "p2")
	if _, err := dispatch.Admit(ctx, r.st, admitReq(a1, "node-1", "owner", true)); err != nil {
		t.Fatal(err)
	}
	// job still running: nothing is torn down, however many sweeps pass.
	r.clk.Advance(10 * time.Minute)
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if r.intent(res.Token).State != store.BurstLive || len(r.cloud.All()) != 1 {
		t.Fatal("a node with a running job must be left alone")
	}
	for _, step := range [][2]model.JobState{{"leased", "running"}, {"running", "finalizing"}, {"finalizing", "passed"}} {
		if _, err := r.eng.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if len(r.cloud.All()) != 1 {
			t.Fatalf("a job in state %s must not be torn down under", step[0])
		}
		_, epoch, err := r.st.AttemptStateEpoch(ctx, a1)
		if err != nil {
			t.Fatal(err)
		}
		if err = r.st.Transition(ctx, a1, step[0], step[1], store.TransitionDetail{Epoch: epoch}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.eng.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.cloud.All()) != 0 || r.intent(res.Token).State != store.BurstTerminal {
		t.Fatal("the job ended, so the machine and the intent end with it")
	}
	if len(r.revoked) != 1 || r.revoked[0] != "node-1" || len(r.denied) != 1 {
		t.Fatalf("NodeRevoke and the deny-list hook: revoked %v denied %v", r.revoked, r.denied)
	}
	// even a late retry of the attempt cannot reuse the node.
	a2 := queue(t, r.st, "p2")
	if _, err := dispatch.Admit(ctx, r.st, admitReq(a2, "node-1", "owner", true)); err == nil {
		t.Fatal("a torn-down node took a second job")
	}
}

func TestEphemeralIdleAndLeaseExpiry(t *testing.T) {
	ctx := context.Background()
	t.Run("joined and never used goes after the idle timeout", func(t *testing.T) {
		r := newRig(t)
		res := r.provision("job-1")
		r.join(res.Token, "node-1")
		r.clk.Advance(4 * time.Minute)
		_, _ = r.eng.Tick(ctx)
		if len(r.cloud.All()) != 1 {
			t.Fatal("torn down before the 5 minute idle timeout")
		}
		r.clk.Advance(2 * time.Minute)
		if _, err := r.eng.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if len(r.cloud.All()) != 0 || len(r.revoked) != 1 {
			t.Fatalf("idle node kept: machines %d revoked %v", len(r.cloud.All()), r.revoked)
		}
	})
	t.Run("never joined goes after the join window", func(t *testing.T) {
		r := newRig(t)
		res := r.provision("job-1")
		r.clk.Advance(6 * time.Minute) // 5 min idle + 120 s default latency = 7 min
		_, _ = r.eng.Tick(ctx)
		if len(r.cloud.All()) != 1 {
			t.Fatal("torn down inside idle timeout + provisioning latency")
		}
		r.clk.Advance(2 * time.Minute)
		if _, err := r.eng.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if len(r.cloud.All()) != 0 || r.intent(res.Token).State != store.BurstTerminal {
			t.Fatal("a machine that never joined must go")
		}
	})
	t.Run("lease expiry ends a busy node", func(t *testing.T) {
		r := newRig(t)
		res := r.provision("job-1")
		r.join(res.Token, "node-1")
		a1 := queue(t, r.st, "p3")
		if _, err := dispatch.Admit(ctx, r.st, admitReq(a1, "node-1", "owner", true)); err != nil {
			t.Fatal(err)
		}
		r.clk.Advance(61 * time.Minute) // max_lifetime
		if _, err := r.eng.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if len(r.cloud.All()) != 0 || len(r.revoked) != 1 {
			t.Fatal("a node past its maximum lifetime must be revoked and deleted even with a job on it")
		}
	})
}

// TestEphemeralStepBindsTheIntentLabel: the budget step lets a runner that carries
// an intent label spend that intent's reservation, so the step must prove the
// runner is the node the intent enrolled.
func TestEphemeralStepBindsTheIntentLabel(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	label := economics.IntentLabelPrefix + res.Token
	a := queue(t, r.st, "p4")
	// another runner claims the intent
	if ref := refusal(t, func() error {
		_, err := dispatch.Admit(ctx, r.st, admitReq(a, "node-evil", "owner", true, label))
		return err
	}()); ref.Reason != model.LabelsMismatch {
		t.Fatalf("a runner that is not the enrolled node: %s", ref.Reason)
	}
	// an unknown intent
	if ref := refusal(t, func() error {
		_, err := dispatch.Admit(ctx, r.st, admitReq(a, "node-1", "owner", true, economics.IntentLabelPrefix+"01JNOSUCHINTENT00000000000"))
		return err
	}()); ref.Reason != model.LabelsMismatch {
		t.Fatalf("a label naming another intent: %s", ref.Reason)
	}
	if _, err := dispatch.Admit(ctx, r.st, admitReq(a, "node-1", "owner", true, label)); err != nil {
		t.Fatalf("the enrolled node with its own label: %v", err)
	}
}

func TestEphemeralStepIgnoresOrdinaryRunners(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	for i := 0; i < 3; i++ {
		a := queue(t, r.st, "p5")
		if _, err := dispatch.Admit(ctx, r.st, admitReq(a, "laptop", "owner", false)); err != nil {
			t.Fatalf("job %d on an ordinary runner: %v", i, err)
		}
	}
	// a local one-job runner with no intent is limited the same way.
	a, b := queue(t, r.st, "p5"), queue(t, r.st, "p5")
	if _, err := dispatch.Admit(ctx, r.st, admitReq(a, "tart-1", "owner", true)); err != nil {
		t.Fatal(err)
	}
	if ref := refusal(t, func() error { _, err := dispatch.Admit(ctx, r.st, admitReq(b, "tart-1", "owner", true)); return err }()); ref.Reason != model.EphemeralUsed {
		t.Fatalf("%s", ref.Reason)
	}
}

func TestBurstStartTearsDownEveryOpenIntent(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a := r.provision("job-a")
	second := r.request("job-b")
	second.QueuedDemand = 2
	b, err := r.eng.Provision(ctx, second)
	if err != nil {
		t.Fatal(err)
	}
	r.join(a.Token, "node-a")
	r.build() // the restarted dispatcher
	if err = r.eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.cloud.All()) != 0 || r.intent(a.Token).State != store.BurstTerminal || r.intent(b.Token).State != store.BurstTerminal {
		t.Fatal("a restarted coordinator keeps no machine of its previous life")
	}
	for _, tok := range []string{a.Token, b.Token} {
		if row := r.resourceRow(tok); row.ConfirmedTeardownAt == nil {
			t.Fatalf("reservation of %s not released", tok)
		}
	}
	if len(r.revoked) != 1 || r.revoked[0] != "node-a" {
		t.Fatalf("only the joined node is revoked: %v", r.revoked)
	}
}

func TestEphemeralNodeBeingTornDownTakesNothing(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	res := r.provision("job-1")
	r.join(res.Token, "node-1")
	if err := r.st.BurstWrite(ctx, func(b store.BurstTx) error { return b.MarkTearingDown(res.Token, "test", b.NowMs()) }); err != nil {
		t.Fatal(err)
	}
	a := queue(t, r.st, "p6")
	_, err := dispatch.Admit(ctx, r.st, admitReq(a, "node-1", "owner", true))
	if ref := refusal(t, err); ref.Reason != model.EphemeralUsed || !strings.Contains(ref.Detail, "torn down") {
		t.Fatalf("%s: %s", ref.Reason, ref.Detail)
	}
}

// TestEphemeralStepRejectsASecondIntentLabel: the budget step reads the first intent label it finds,
// so a runner may not carry another intent's label in any position.
func TestEphemeralStepRejectsASecondIntentLabel(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	mine := r.provision("job-1")
	r.join(mine.Token, "node-1")
	other := r.request("job-2")
	other.QueuedDemand = 2
	victim, err := r.eng.Provision(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	own, theirs := economics.IntentLabelPrefix+mine.Token, economics.IntentLabelPrefix+victim.Token
	for _, labels := range [][]string{{theirs, own}, {own, theirs}, {theirs}} {
		a := queue(t, r.st, "p7")
		_, err := dispatch.Admit(ctx, r.st, admitReq(a, "node-1", "owner", true, labels...))
		if ref := refusal(t, err); ref.Reason != model.LabelsMismatch {
			t.Fatalf("labels %v: %s", labels, ref.Reason)
		}
	}
	a := queue(t, r.st, "p7")
	if _, err = dispatch.Admit(ctx, r.st, admitReq(a, "node-1", "owner", true, own, own)); err != nil {
		t.Fatalf("the same own label twice is harmless: %v", err)
	}
}
