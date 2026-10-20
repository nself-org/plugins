//go:build integration

package reliability

// Purpose: the coordinator is the one that goes away (D6 semantics). A restart in the middle of a job's output
//          loses and duplicates no line and requeues nothing. An outage shorter than agent_ttl with twenty running
//          agents requeues nothing. An outage of five minutes makes every agent fence itself; each attempt then
//          goes lost as agent.self_fenced, is retried exactly once, its side effect is written once per job, and
//          the coordinator expires no lease for its own downtime.

import (
	"strings"
	"testing"
	"time"
)

func TestFaultCoordinatorRestartMidLogStream(t *testing.T) {
	l := newLab(t, outage, 3)
	const iter = 100
	h := l.start(jobSpec{iter: iter, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	fi := l.index(h.runnerOf(atts[0].ID))
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to be midway", func() bool { return l.lines(fi, "/tmp/beats") >= 40 }, h.diagText)

	l.c.down() // kill -9 of the listener while the job is writing
	hold(3 * time.Second)
	l.c.up()
	l.waitConnected(30*time.Second, l.ids[fi])

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 1 || len(h.attempts()) != 1 {
		t.Fatalf("a coordinator restart caused a retry: exit %d jobs %+v attempts %+v\n%s", res.Exit, res.Summary.Jobs, h.attempts(), h.out)
	}
	assertTicks(t, h.leaseLog(atts[0].ID), iter)
	if n := l.fleetSideEffects(); n != 1 {
		t.Fatalf("the side effect was written %d times, want 1", n)
	}
}

// busyFleet starts count jobs, one per node, and waits until every job is running and every lease was renewed.
func busyFleet(t *testing.T, p timing, count int, iter, retryIter int) (*lab, *runHandle) {
	t.Helper()
	l := newLab(t, p, count)
	h := l.startJobs(count, jobSpec{iter: iter, retryIter: retryIter, idempotent: true, infraMax: 1})
	until(t, 8*time.Minute, "every job to be running on its own node", func() bool {
		running := 0
		for _, a := range h.attempts() {
			if a.State == "leased" || a.State == "running" {
				running++
			}
		}
		return running == count
	}, h.diagText)
	for _, a := range h.attempts() {
		h.waitRenewed(a.ID)
	}
	for i := range l.names {
		i := i
		until(t, 60*time.Second, "node "+l.names[i]+" to run its job", func() bool { return l.lines(i, "/tmp/beats") >= 3 }, h.diagText)
	}
	return l, h
}

func TestFaultCoordinatorDownBelowAgentTTL(t *testing.T) {
	l, h := busyFleet(t, outage, 20, 1000, 20)
	before := len(h.attempts())

	l.c.down()
	hold(20 * time.Second) // < agent_ttl (56 s)
	l.c.up()
	l.waitConnected(60*time.Second, l.ids...)

	res := h.result(4 * time.Minute)
	if res.Exit != 0 || len(res.Summary.Jobs) != 20 {
		t.Fatalf("exit %d, %d jobs\n%s", res.Exit, len(res.Summary.Jobs), h.out)
	}
	if after := len(h.attempts()); after != before || after != 20 {
		t.Fatalf("%d attempts before the outage, %d after, want 20 and no requeue; retried: %v", before, after, l.auditDetails("retry-queued"))
	}
	if got := l.auditDetails("retry-queued"); len(got) != 0 {
		t.Fatalf("the outage requeued %d attempts: %v", len(got), got)
	}
	for name, n := range l.fleetSideEffectsByJob() {
		if n != 1 {
			t.Errorf("job %s wrote its side effect %d times", name, n)
		}
	}
}

func TestFaultCoordinatorDownFiveMinutes(t *testing.T) {
	l, h := busyFleet(t, outage, 20, 3000, 20)
	l.c.down()
	// every agent fences itself: its job process is gone long before the coordinator is back
	until(t, outage.agentTTL()+2*time.Minute, "every agent to kill its job", func() bool {
		for i := range l.names {
			if l.jobAlive(i) {
				return false
			}
		}
		return true
	}, h.diagText)
	hold(5*time.Minute - outage.agentTTL() - 5*time.Second)
	l.c.up()
	l.waitConnected(2*time.Minute, l.ids...)

	res := h.result(6 * time.Minute)
	if res.Exit != 0 || len(res.Summary.Jobs) != 20 {
		t.Fatalf("exit %d, %d jobs\n%s", res.Exit, len(res.Summary.Jobs), h.out)
	}
	retried := l.auditDetails("retry-queued")
	if len(retried) != 20 {
		t.Fatalf("%d attempts were retried, want 20 (one per job): %v\n%s", len(retried), retried, l.auditSummary())
	}
	for _, r := range retried {
		if !strings.Contains(r, "agent.self_fenced") {
			t.Errorf("an attempt was lost for another reason than the agent's own fence: %s", r)
		}
	}
	for _, j := range res.Summary.Jobs {
		if j.N != 2 || j.State != "passed" {
			t.Errorf("job %s ended as attempt %d %s, want attempt 2 passed", j.ID, j.N, j.State)
		}
	}
	for name, n := range l.fleetSideEffectsByJob() {
		if n != 1 {
			t.Errorf("job %s wrote its side effect %d times", name, n)
		}
	}
}
