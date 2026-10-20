//go:build integration

package reliability

// Purpose: scenario 9, first half: a node whose processes are frozen (docker pause, the SIGSTOP of the whole node)
//          past the lease TTL. The agent cannot fence while frozen; the coordinator expires the lease, the attempt
//          goes lost and is retried on another node with reason retry.anti_affinity; when the node thaws the stale
//          job is killed at once and its late traffic is refused, so the side effect is written once.

import (
	"strings"
	"testing"
	"time"
)

func TestFaultFrozenNodePastTTL(t *testing.T) {
	l := newLab(t, fast, 3)
	h := l.start(jobSpec{iter: 100, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	original := atts[0]
	fi := l.index(first)
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)

	l.pause(fi)
	frozenAt := time.Now()
	h.waitAttempt(1, 3*fast.ttl+10*time.Second, "lost", "retryable", "queued", "cancelled")
	if took := time.Since(frozenAt); took < fast.ttl-2*fast.heartbeat {
		t.Fatalf("attempt 1 was lost %v after the freeze, before the lease could expire (ttl %v)", took, fast.ttl)
	}
	atts = h.waitAttempt(2, 30*time.Second, "leased", "running")
	second := h.runnerOf(atts[len(atts)-1].ID)
	if second == first {
		t.Fatalf("the retry went back to the frozen node %s", first)
	}
	if got := h.previousOf(atts[len(atts)-1].ID); got != first {
		t.Fatalf("the retry recorded previous runner %q, want %s", got, first)
	}
	if doc := h.decision(atts[len(atts)-1].ID); !strings.Contains(doc, "retry.anti_affinity") {
		t.Fatalf("the placement decision of the retry does not name retry.anti_affinity:\n%s", doc)
	}

	l.unpause(fi)
	thawed := time.Now()
	until(t, fast.agentTTL()+10*time.Second, "the thawed node's stale job to die", func() bool { return !l.jobAlive(fi) }, h.diagText)
	if took := time.Since(thawed); took > fast.agentTTL()+5*time.Second {
		t.Fatalf("the stale job lived %v after the node thawed", took)
	}
	l.waitConnected(30*time.Second, first)
	l.assertStaleTraffic(original.ID, original.Epoch) // the fenced agent is back and takes work again

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || len(res.Summary.Jobs) != 1 || res.Summary.Jobs[0].N != 2 {
		t.Fatalf("exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	retries := res.Summary.Doc.Retries
	if len(retries) != 1 || retries[0].Attempt != 2 || retries[0].Class != "infra" {
		t.Fatalf("evidence retries = %+v, want one infra retry into attempt 2", retries)
	}
	if n := l.fleetSideEffects(); n != 1 {
		t.Fatalf("the side effect was written %d times, want 1", n)
	}
	if n := l.sideEffects(fi); n != 0 {
		t.Fatalf("the frozen node wrote the side effect %d times", n)
	}
}
