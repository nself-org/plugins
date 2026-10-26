package reliability

// Purpose: scenario 9. A node frozen (SIGSTOP / docker pause) or partitioned past the TTL self-fences
//          before the coordinator's expiry; the attempt goes lost and is retried on another eligible
//          node (retry.anti_affinity); the late result after SIGCONT or heal is rejected with E662; the
//          side-effect file is written once; the evidence lists both attempts with failure_class.
//          A deterministic failure is not retried; a lost release job goes needs-operator.
// Inputs:  the sim world (real store, lease engine, agent fence, scheduler) with injected clocks.
// Outputs: assertions only.
// Constraints: no sleeps; the Docker variant of the same faults is docker_faults_test.go.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// retryElsewhere checks the retry placement and runs the retry to completion on node-b.
func retryElsewhere(t *testing.T, w *world, jobID string, rows []store.AttemptView) *agent {
	t.Helper()
	prev := w.previousRunner(rows[1].ID)
	if prev != "node-a" {
		t.Fatalf("retry anti-affinity runner = %q, want node-a", prev)
	}
	res := place(jobID, rows[1].N, prev, simRunner("node-a"), simRunner("node-b"))
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "node-b" {
		t.Fatalf("retry placement = %+v, want node-b", res.Placements)
	}
	if got := reasonsFor(res, "node-a"); !contains(got, string(model.RetryAntiAffinity)) {
		t.Fatalf("node-a reasons = %v, want %s", got, model.RetryAntiAffinity)
	}
	b := w.startAgent("node-b", "lease-b", w.grant(rows[1].ID, "lease-b", "node-b", 1))
	w.run(heartbeat, b)
	if ran, err := b.finish(jobID); !ran || err != nil {
		t.Fatalf("retry on node-b: ran=%t err=%v", ran, err)
	}
	return b
}

// fencedBeforeExpiry asserts the agent's fence deadline falls before the coordinator's lease expiry.
func fencedBeforeExpiry(t *testing.T, a *agent, expiresAt int64) {
	t.Helper()
	deadline := a.lastAck.Add(a.fence.TTL)
	if !deadline.Before(time.Unix(0, expiresAt)) {
		t.Fatalf("agent fence deadline %v not before coordinator expiry %v", deadline, time.Unix(0, expiresAt))
	}
	if !a.fencedAt.IsZero() && !a.fencedAt.Before(time.Unix(0, expiresAt)) {
		t.Fatalf("agent fenced at %v, after coordinator expiry %v", a.fencedAt, time.Unix(0, expiresAt))
	}
}

func TestFaultScenario9Freeze(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("j9", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(2*heartbeat, a)
	l, err := w.store.LeaseGet(w.ctx, "lease-a")
	if err != nil {
		t.Fatal(err)
	}
	a.frozen = true // SIGSTOP
	w.run(time.Minute, a)
	fencedBeforeExpiry(t, a, l.ExpiresAt)
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 1 {
		t.Fatalf("sweep after freeze: %d %v", n, err)
	}
	a.frozen = false // SIGCONT: the wake check latches the fence before job work resumes
	if ran, _ := a.finish("j9"); ran || a.fencedAt.IsZero() {
		t.Fatalf("frozen job resumed: ran=%t fencedAt=%v", ran, a.fencedAt)
	}
	wantCode(t, "late result after SIGCONT", a.lateResult(), "E662")
	rows := w.wantAttempts("j9", "retryable/lost", "queued/")
	if got := w.retryReasons(a1); len(got) != 1 || got[0] != "lease.expired" {
		t.Fatalf("retry reasons %v", got)
	}
	retryElsewhere(t, w, "j9", rows)
	wantCode(t, "late result after the retry", a.lateResult(), "E662")
	w.wantAttempts("j9", "retryable/lost", "finalizing/")
	w.wantEffect("j9", "node-b")
	if w.engine.StaleCount() != 2 {
		t.Fatalf("stale rejections = %d, want 2", w.engine.StaleCount())
	}
}

func TestFaultScenario9Partition(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("j9p", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(2*heartbeat, a)
	l, err := w.store.LeaseGet(w.ctx, "lease-a")
	if err != nil {
		t.Fatal(err)
	}
	a.partitioned = true // netem loss 100 % / network disconnect
	w.run(3*heartbeat, a)
	if a.fencedAt.IsZero() {
		t.Fatal("partitioned agent did not self-fence")
	}
	fencedBeforeExpiry(t, a, l.ExpiresAt)
	if ran, _ := a.finish("j9p"); ran {
		t.Fatal("fenced job wrote its side effect")
	}
	a.partitioned = false // heal: the agent reports its fence
	if err := a.reconnect(); err != nil {
		t.Fatal(err)
	}
	if got := w.retryReasons(a1); len(got) != 1 || got[0] != "agent.self_fenced" {
		t.Fatalf("retry reasons %v", got)
	}
	wantCode(t, "late result after heal", a.lateResult(), "E662")
	wantCode(t, "second fence report", a.reconnect(), "E662")
	w.advance(coordTTL)
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 0 {
		t.Fatalf("sweep after self-fence: %d %v", n, err)
	}
	rows := w.wantAttempts("j9p", "retryable/lost", "queued/")
	retryElsewhere(t, w, "j9p", rows)
	w.wantAttempts("j9p", "retryable/lost", "finalizing/")
	w.wantEffect("j9p", "node-b")
}

func TestFaultDeterministicNotRetried(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("jdet", "test", true, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(heartbeat, a)
	w.effect("jdet", "node-a")
	_, epoch := w.state(a1)
	if err := w.store.Transition(w.ctx, a1, "running", "failed", store.TransitionDetail{Epoch: epoch, FailureClass: "code"}); err != nil {
		t.Fatal(err)
	}
	w.advance(2 * coordTTL)
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 0 {
		t.Fatalf("deterministic failure swept: %d %v", n, err)
	}
	w.wantAttempts("jdet", "failed/code")
	w.wantEffect("jdet", "node-a")
	if got := w.retryReasons(a1); len(got) != 0 {
		t.Fatalf("deterministic failure retried: %v", got)
	}
}

func TestFaultLostReleaseNeedsOperator(t *testing.T) {
	w := newWorld(t)
	a1 := w.job("jrel", "release", false, 1)
	a := w.startAgent("node-a", "lease-a", w.grant(a1, "lease-a", "node-a", 1))
	w.run(heartbeat, a)
	a.dead = true // kill -9 of the agent running the release
	w.run(2*coordTTL, a)
	if n, err := w.engine.Sweep(w.ctx); err != nil || n != 1 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	w.wantAttempts("jrel", "needs-operator/lost")
	if got := w.retryReasons(a1); len(got) != 0 {
		t.Fatalf("release retried: %v", got)
	}
	if lines := w.effectLines("jrel"); len(lines) != 0 {
		t.Fatalf("release side effect %v", lines)
	}
}
