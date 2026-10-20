//go:build integration

package reliability

// Purpose: cancels under fault. A cancel issued while the node is cut off ends the attempt at once in the store
//          (no retry), and the job dies when the node is back. A cancel that races the job's own result leaves
//          exactly one terminal state, no retry and at most one side effect.

import (
	"testing"
	"time"
)

func TestFaultCancelWhileDisconnected(t *testing.T) {
	l := newLab(t, outage, 3)
	h := l.start(jobSpec{iter: 400, idempotent: true, infraMax: 1})
	atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
	fi := l.index(h.runnerOf(atts[0].ID))
	h.waitRenewed(atts[0].ID)
	until(t, 30*time.Second, "the job to beat", func() bool { return l.lines(fi, "/tmp/beats") >= 3 }, h.diagText)

	l.partition(fi)
	until(t, 20*time.Second, "the node to be cut off", func() bool { return !l.agentReachable(fi) }, nil)
	h.stop() // the operator cancels the run while the node cannot hear it

	res := h.result(2 * time.Minute)
	if res.Exit == 0 {
		t.Fatalf("a cancelled run exited 0\n%s", h.out)
	}
	atts = h.attempts()
	if len(atts) != 1 || atts[0].State != "cancelled" {
		t.Fatalf("the attempts after a cancel are %+v, want one cancelled attempt and no retry", atts)
	}
	if !l.jobAlive(fi) {
		t.Fatal("the job ended before the node could hear the cancel; the fixture shows nothing")
	}
	l.heal(fi)
	l.waitConnected(60*time.Second, l.ids[fi])
	until(t, 30*time.Second, "the node to stop the cancelled job once it hears the coordinator", func() bool { return !l.jobAlive(fi) }, h.diagText)
	if n := l.fleetSideEffects(); n != 0 {
		t.Fatalf("a cancelled job wrote its side effect %d times", n)
	}
	if got := len(h.attempts()); got != 1 {
		t.Fatalf("%d attempts after the node came back, want 1", got)
	}
}

func TestFaultCancelRacesResult(t *testing.T) {
	l := newLab(t, fast, 2)
	const iter = 25
	for round := 0; round < 4; round++ {
		l.resetBeats()
		before := l.fleetSideEffects()
		h := l.start(jobSpec{iter: iter, idempotent: true, infraMax: 1})
		atts := h.waitAttempt(1, 60*time.Second, "leased", "running")
		fi := l.index(h.runnerOf(atts[0].ID))
		// cancel when the job is `round` ticks from its end: the result and the cancel cross
		until(t, 60*time.Second, "the job to near its end", func() bool { return l.lines(fi, "/tmp/beats") >= iter-round }, h.diagText)
		h.stop()
		res := h.result(time.Minute)
		got := h.attempts()
		if len(got) != 1 {
			t.Fatalf("round %d: %d attempts after a cancel race, want 1: %+v", round, len(got), got)
		}
		if st := got[0].State; st != "passed" && st != "cancelled" {
			t.Fatalf("round %d: the attempt ended %s, want passed or cancelled (exit %d)\n%s", round, st, res.Exit, h.out)
		}
		if (got[0].State == "passed") != (res.Exit == 0) {
			t.Fatalf("round %d: attempt %s but run exit %d", round, got[0].State, res.Exit)
		}
		until(t, 30*time.Second, "the job to be gone", func() bool { return !l.jobAlive(fi) }, h.diagText)
		if added := l.fleetSideEffects() - before; added > 1 {
			t.Fatalf("round %d: the side effect was written %d times", round, added)
		}
	}
}
