//go:build integration

package reliability

// Purpose: the ssh carrier under session loss. When the session drops mid-job the attempt is lost (no resume is
//          claimed), the agent kills its job, and the attempt is retried on another node; the side effect is
//          written once.

import (
	"testing"
	"time"
)

func TestFaultSSHSessionLoss(t *testing.T) {
	l := newSSHLab(t, 2)
	h := l.start(jobSpec{iter: 100, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 2*time.Minute, "leased", "running")
	first := h.runnerOf(atts[0].ID)
	fi := l.index(first)
	until(t, 60*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)

	l.killSSHSession(fi)
	h.waitAttempt(1, 60*time.Second, "lost", "retryable", "queued", "cancelled")
	atts = h.waitAttempt(2, 60*time.Second, "leased", "running")
	retry := atts[len(atts)-1].ID
	if h.runnerOf(retry) == first {
		t.Fatalf("the retry went back to the node whose session dropped (%s)", first)
	}
	until(t, 30*time.Second, "the agent to kill the job of the lost session", func() bool { return !l.jobAlive(fi) }, h.diagText)

	res := h.result(2 * time.Minute)
	if res.Exit != 0 || res.Summary.Jobs[0].N != 2 {
		t.Fatalf("exit %d jobs %+v\n%s", res.Exit, res.Summary.Jobs, h.out)
	}
	if n := l.fleetSideEffects(); n != 1 || l.sideEffects(fi) != 0 {
		t.Fatalf("the side effect was written %d times (%d on the node that lost its session)", n, l.sideEffects(fi))
	}
}
