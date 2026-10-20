//go:build integration

package reliability

// Purpose: crashes of the agent process and of the whole node. kill -9 of the agent: its job is an orphan, the
//          lease expires, the attempt is retried on another node and the restarted agent kills the orphan before
//          it can write the side effect. A container restart (a VPS reboot) loses the job with the node; the node
//          comes back, its agent is started again and takes work.

import (
	"testing"
	"time"
)

func TestFaultAgentKill9(t *testing.T) {
	l := newLab(t, fast, 3)
	h := l.start(jobSpec{iter: 100, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	fi := l.index(first)
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)

	l.killAgent(fi)
	until(t, 10*time.Second, "the agent to be gone", func() bool { return !l.agentAlive(fi) }, h.diagText)
	if !l.jobAlive(fi) {
		t.Fatal("the job died with the agent: the fixture cannot show an orphan")
	}
	h.waitAttempt(1, 3*fast.ttl+10*time.Second, "lost", "retryable", "queued", "cancelled")
	atts = h.waitAttempt(2, 30*time.Second, "leased", "running")
	retry := atts[len(atts)-1].ID
	if h.runnerOf(retry) == first {
		t.Fatalf("the retry went back to the node whose agent died (%s)", first)
	}

	l.startAgent(fi) // the supervisor restarts the agent
	l.waitConnected(30*time.Second, first)
	restarted := time.Now()
	until(t, 30*time.Second, "the restarted agent to kill the orphaned job", func() bool { return !l.jobAlive(fi) }, h.diagText)
	t.Logf("the orphan died %v after the agent restarted", time.Since(restarted))

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 2 {
		t.Fatalf("exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	if n := l.fleetSideEffects(); n != 1 || l.sideEffects(fi) != 0 {
		t.Fatalf("the side effect was written %d times (%d by the orphan), want once on another node", n, l.sideEffects(fi))
	}
}
