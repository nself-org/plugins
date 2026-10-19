package dispatch

import (
	"context"
	"strings"
	"testing"
	"time"
)

var queuedSeq uint64

func queuedAt(clock *fakeClock, project, id, band, priority string) Queued {
	j := jobIn(project, id)
	j.Sched.Pipeline.Band, j.Sched.Priority = band, priority
	queuedSeq++
	return Queued{Job: j, EnqueuedAt: clock.Now(), Seq: queuedSeq}
}

func ids(q []Queued) string {
	var out []string
	for _, e := range q {
		out = append(out, e.Job.AttemptID)
	}
	return strings.Join(out, " ")
}

// TestFairOrder: strict bands, projects interleave, priority then age inside a project.
func TestFairOrder(t *testing.T) {
	clock := newClock()
	f := NewFair(nil, clock.Now)
	var q []Queued
	for _, e := range []struct{ project, id, band, prio string }{
		{"alpha", "a1", "background", ""}, {"alpha", "a2", "", ""}, {"alpha", "a3", "", ""}, {"beta", "b1", "", ""},
		{"alpha", "a4", "", "high"}, {"beta", "b2", "interactive", ""},
	} {
		q = append(q, queuedAt(clock, e.project, e.id, e.band, e.prio))
		clock.Advance(time.Second)
	}
	// interactive first; then normal: alpha and beta alternate (alpha's high-priority job leads its own project); background last
	if got := ids(f.Order(q)); got != "b2 a4 b1 a2 a3 a1" {
		t.Fatalf("order = %s", got)
	}
	if got := ids(NewFair(nil, clock.Now).Order(q)); got != "b2 a4 b1 a2 a3 a1" {
		t.Fatalf("a second orderer gave %s: the order must be deterministic", got)
	}
	// a foreground job is interactive without a declared band
	fg := queuedAt(clock, "alpha", "f1", "", "")
	fg.Job.Foreground = true
	if got := ids(NewFair(nil, clock.Now).Order([]Queued{q[1], fg})); got != "f1 a2" {
		t.Fatalf("foreground order = %s", got)
	}
}

// TestFairOrderStarvation: a background job that waited past the ceiling is
// ordered with the normal jobs of its project by age, and never ahead of interactive.
func TestFairOrderStarvation(t *testing.T) {
	clock := newClock()
	limits := func() QueueLimits { return QueueLimits{StarvationAfterMs: 30 * 60 * 1000} }
	f := NewFair(limits, clock.Now)
	bg := queuedAt(clock, "alpha", "bg1", "background", "")
	clock.Advance(29 * time.Minute)
	young := queuedAt(clock, "alpha", "n1", "", "")
	ia := queuedAt(clock, "beta", "i1", "interactive", "")
	if got := ids(f.Order([]Queued{young, bg, ia})); got != "i1 n1 bg1" {
		t.Fatalf("before the ceiling: %s", got)
	}
	clock.Advance(2 * time.Minute) // bg1 has now waited 31 minutes
	if got := ids(f.Order([]Queued{young, bg, ia})); got != "i1 bg1 n1" {
		t.Fatalf("after the ceiling the older job leads its project: %s", got)
	}
}

// TestFairOrderVirtualTime: a project that was just served waits behind an idle one.
func TestFairOrderVirtualTime(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	local := testRunner("local", "local", 1)
	local.Capability.Lifecycle.AuthorizedProjects = []string{"alpha", "beta"}
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, local), Clock: clock.Now})
	var es []*entry
	for _, e := range []struct{ project, name string }{{"alpha", "a1"}, {"alpha", "a2"}, {"alpha", "a3"}, {"beta", "b1"}} {
		id := queueAttempt(t, st, "pipe-"+e.project, e.project, e.name)
		j := jobIn(e.project, id)
		j.Sched.Pipeline.ID = "pipe-" + e.project
		en, err := d.enqueue(j)
		if err != nil {
			t.Fatal(err)
		}
		es = append(es, en)
		clock.Advance(time.Second)
	}
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(es[0]) || resolved(es[1]) || resolved(es[3]) {
		t.Fatalf("one slot: alpha's first job takes it: %v %v %v", resolved(es[0]), resolved(es[1]), resolved(es[3]))
	}
	pass(t, st, es[0].res.AttemptID)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(es[3]) || resolved(es[1]) {
		t.Fatalf("beta's job must pass alpha's second one: beta %v alpha %v", resolved(es[3]), resolved(es[1]))
	}
}

// TestPriorityNeverAdmits: a high-priority interactive job no runner can take
// stays queued and does not block the jobs behind it.
func TestPriorityNeverAdmits(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	sink := &recSink{}
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2)), Clock: clock.Now, Sink: sink})
	ghost := queueAttempt(t, st, "pipe", "project", "ghost")
	plain := queueAttempt(t, st, "pipe", "project", "plain")
	gj := jobIn("project", ghost)
	gj.Sched.Pipeline.Band, gj.Sched.Priority = "interactive", "high"
	gj.Sched.Requirements.Platform = "plan9/riscv"
	gj.Sched.Pipeline.Support = []string{"plan9/riscv"}
	ge, err := d.enqueue(gj)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	pe, err := d.enqueue(jobIn("project", plain))
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if resolved(ge) || !resolved(pe) {
		t.Fatalf("the ineligible job stays queued (%v) and the plain one runs (%v)", resolved(ge), resolved(pe))
	}
	for _, dec := range sink.all() {
		if dec.Job.Key == ghost && dec.Outcome == OutcomeAdmitted {
			t.Fatal("priority admitted a job no runner may take")
		}
	}
}
