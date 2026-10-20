//go:build integration

package reliability

// Purpose: faults of the node as a whole and the rules that follow a lost job. A node reboot (the container
//          restart of a VPS reboot) loses the agent and the job; a black-hole node that never answers fails
//          three jobs in a row and goes to maintenance; a deterministic failure is not retried.

import (
	"testing"
	"time"
)

func TestFaultNodeReboot(t *testing.T) {
	l := newLab(t, fast, 3)
	h := l.start(jobSpec{iter: 100, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	fi := l.index(first)
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)

	l.f.Restart(t, l.names[fi]) // the node reboots: no agent, no job
	if l.jobAlive(fi) || l.agentAlive(fi) {
		t.Fatal("a process survived the node reboot")
	}
	h.waitAttempt(1, 3*fast.ttl+10*time.Second, "lost", "retryable", "queued", "cancelled")
	atts = h.waitAttempt(2, 30*time.Second, "leased", "running")
	if h.runnerOf(atts[len(atts)-1].ID) == first {
		t.Fatalf("the retry went to the node that was rebooting (%s)", first)
	}
	l.startAgent(fi) // the service manager starts the agent when the node is back
	l.waitConnected(60*time.Second, first)

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 2 {
		t.Fatalf("exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	if n := l.fleetSideEffects(); n != 1 || l.sideEffects(fi) != 0 {
		t.Fatalf("the side effect was written %d times (%d on the rebooted node), want once elsewhere", n, l.sideEffects(fi))
	}
}

func TestFaultDeterministicFailureIsNotRetried(t *testing.T) {
	l := newLab(t, fast, 3)
	h := l.start(jobSpec{iter: 10, idempotent: true, infraMax: 2, failExit: 3})
	res := h.result(2 * time.Minute)
	if res.Exit == 0 || len(res.Summary.Jobs) != 1 {
		t.Fatalf("a failing job passed: exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	j := res.Summary.Jobs[0]
	if j.N != 1 || j.State != "failed" || j.FailureClass != "code" || len(j.Retries) != 0 {
		t.Fatalf("a deterministic failure was retried or misclassified: attempt %d state %s class %s retries %+v", j.N, j.State, j.FailureClass, j.Retries)
	}
	if got := len(h.attempts()); got != 1 {
		t.Fatalf("%d attempts, want 1: %+v", got, h.attempts())
	}
	if got := l.auditDetails("retry-queued"); len(got) != 0 {
		t.Fatalf("the failure was queued for a retry: %v", got)
	}
	if n := l.fleetSideEffects(); n != 0 {
		t.Fatalf("a failed job wrote its side effect %d times", n)
	}
}

func TestFaultBlackHoleNodeGoesToMaintenance(t *testing.T) {
	l := newLab(t, fast, 2)
	l.setNodeState(1, "maintenance") // only the black hole can take the job
	l.f.Netem(t, l.names[0], 0, 100) // it answers nothing: every packet it sends is dropped
	h := l.start(jobSpec{iter: 10, idempotent: true, infraMax: 2})

	// three attempts lose their lease on the black hole, one after the other (the grant's own TTL, no heartbeat)
	until(t, 8*time.Minute, "the black hole to be put in maintenance", func() bool { return l.nodeState(0) == "maintenance" }, h.diagText)
	res := h.result(3 * time.Minute)
	if res.Exit == 0 {
		t.Fatalf("a run on a black hole passed\n%s", h.out)
	}
	lost := 0
	for _, a := range h.attempts() {
		if a.FailureClass == "lost" || a.State == "lost" || a.State == "retryable" {
			lost++
		}
	}
	if lost < 3 {
		t.Fatalf("%d attempts were lost on the black hole, want 3: %+v", lost, h.attempts())
	}
	if n := l.fleetSideEffects(); n != 0 {
		t.Fatalf("the black hole's job wrote its side effect %d times", n)
	}

	// the operator re-enables the healthy node: new work goes there, never to the black hole
	l.setNodeState(1, "online")
	h2 := l.start(jobSpec{iter: 10, idempotent: true, infraMax: 1})
	res = h2.result(2 * time.Minute)
	if res.Exit != 0 || l.sideEffects(1) != 1 || l.sideEffects(0) != 0 {
		t.Fatalf("the run after maintenance: exit %d, side effects n1=%d n2=%d\n%s", res.Exit, l.sideEffects(0), l.sideEffects(1), h2.out)
	}
}
