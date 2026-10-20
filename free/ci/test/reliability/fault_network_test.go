//go:build integration

package reliability

// Purpose: network faults between a node and the coordinator. A partition that outlasts the TTL: the agent
//          fences first (its job dies before the lease deadline), the coordinator then expires the lease and the
//          attempt is retried on another node. A short drop (a Wi-Fi blip) inside the agent TTL: nothing is lost,
//          requeued or duplicated; the job's output arrives once, in order.

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFaultPartitionPastTTL(t *testing.T) {
	l := newLab(t, fast, 3)
	h := l.start(jobSpec{iter: 100, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	original := atts[0]
	fi := l.index(first)
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)
	le, err := l.c.st.LeaseGet(context.Background(), "lease-"+atts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Unix(0, le.ExpiresAt).Add(fast.heartbeat) // one more heartbeat may land before the cut

	l.partition(fi)
	h.waitAttempt(1, 3*fast.ttl+10*time.Second, "lost", "retryable", "queued", "cancelled")
	atts = h.waitAttempt(2, 30*time.Second, "leased", "running")
	retry := atts[len(atts)-1].ID
	if h.runnerOf(retry) == first || h.previousOf(retry) != first {
		t.Fatalf("the retry ran on %s (previous %s), want a node other than %s", h.runnerOf(retry), h.previousOf(retry), first)
	}
	if doc := h.decision(retry); !strings.Contains(doc, "retry.anti_affinity") {
		t.Fatalf("the placement decision of the retry does not name retry.anti_affinity:\n%s", doc)
	}
	// the agent fenced before the coordinator expired the lease: the cut-off node's job is gone, and its last
	// beat is older than the deadline the lease had when the network went away
	until(t, 20*time.Second, "the cut-off node's job to be killed by its agent", func() bool { return !l.jobAlive(fi) }, h.diagText)
	if beat := l.lastBeat(fi); beat.After(deadline) {
		t.Fatalf("the cut-off job still ran at %v, after the lease deadline %v: the agent did not fence first", beat, deadline)
	}
	l.heal(fi)
	l.waitConnected(30*time.Second, first)
	l.assertStaleTraffic(original.ID, original.Epoch)

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 2 {
		t.Fatalf("exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	if n := l.fleetSideEffects(); n != 1 || l.sideEffects(fi) != 0 {
		t.Fatalf("the side effect was written %d times (%d on the cut-off node), want once elsewhere", n, l.sideEffects(fi))
	}
}

func TestFaultNetworkDropWithinAgentTTL(t *testing.T) {
	l := newLab(t, outage, 3)
	const iter = 80
	h := l.start(jobSpec{iter: iter, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	fi := l.index(first)
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 10 }, h.diagText)

	l.partition(fi)
	until(t, 20*time.Second, "the node to be cut off", func() bool { return !l.agentReachable(fi) }, nil)
	until(t, 30*time.Second, "the coordinator to hear nothing from the node for 3 s", func() bool { return time.Since(l.c.lastAck(first)) >= 3*time.Second }, nil)
	l.heal(fi)

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 1 {
		t.Fatalf("a short drop caused a retry: exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	if got := len(h.attempts()); got != 1 {
		t.Fatalf("%d attempts after a short drop, want 1: %+v", got, h.attempts())
	}
	assertTicks(t, h.leaseLog(atts[0].ID), iter)
	if n := l.fleetSideEffects(); n != 1 || l.sideEffects(fi) != 1 {
		t.Fatalf("the side effect was written %d times (%d on the node that ran it)", n, l.sideEffects(fi))
	}
}
