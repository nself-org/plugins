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
	l.setNodeState(1, "maintenance") // only the black hole can take the first attempts
	l.f.Netem(t, l.names[0], 0, 100) // it answers nothing: every packet it sends is dropped
	h := l.startJobs(3, jobSpec{iter: 10, idempotent: true, infraMax: 1})

	// three jobs lose their lease on the black hole (the grant's own TTL, no heartbeat): three infra failures in a row
	until(t, 8*time.Minute, "the black hole to be put in maintenance", func() bool { return l.nodeState(0) == "maintenance" }, h.diagText)
	lost := 0
	for _, a := range h.attempts() {
		if a.FailureClass == "lost" && a.N == 1 {
			lost++
		}
	}
	if lost < 3 {
		t.Fatalf("%d attempts were lost on the black hole, want 3: %+v", lost, h.attempts())
	}
	if n := l.fleetSideEffects(); n != 0 {
		t.Fatalf("the black hole's jobs wrote their side effect %d times", n)
	}

	// the operator brings the healthy node back: the retries go there (anti-affinity), never to the black hole
	l.setNodeState(1, "online")
	res := h.result(3 * time.Minute)
	if res.Exit != 0 || len(res.Summary.Jobs) != 3 {
		t.Fatalf("exit %d, %d jobs\n%s", res.Exit, len(res.Summary.Jobs), h.out)
	}
	for _, j := range res.Summary.Jobs {
		if j.N != 2 || j.State != "passed" {
			t.Errorf("job %s ended as attempt %d %s, want attempt 2 passed", j.ID, j.N, j.State)
		}
	}
	if l.sideEffects(0) != 0 || l.sideEffects(1) != 3 || l.nodeState(0) != "maintenance" {
		t.Fatalf("side effects n1=%d n2=%d, black hole state %s; want 0, 3, maintenance", l.sideEffects(0), l.sideEffects(1), l.nodeState(0))
	}
}
