package dispatch

// Interleavings of the tick with callers that give up, with Close and with
// deadlines, and the order guarantee across pipelines.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestDispatchOrderAcrossPipelines: admission follows the Orderer's order across
// pipelines; a younger job of pipeline A never passes an older job of pipeline B.
func TestDispatchOrderAcrossPipelines(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2)), Clock: clock.Now})
	mk := func(pipe, name string) *entry {
		id := queueAttempt(t, st, pipe, "proj", name)
		j := testQueued(id)
		j.Sched.Pipeline.ID = pipe
		e, err := d.enqueue(j)
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Second)
		return e
	}
	a1, b1, a2 := mk("A", "one"), mk("B", "one"), mk("A", "two")
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(a1) || !resolved(b1) || resolved(a2) {
		t.Fatalf("2 slots, queue A1 B1 A2: admitted A1=%v B1=%v A2=%v", resolved(a1), resolved(b1), resolved(a2))
	}
}

// TestDispatchCancelDuringAdmit: a caller that gives up while ANOTHER goroutine's
// tick is committing its lease waits for the commit and gets the lease back,
// instead of leaving a lease with no owner.
func TestDispatchCancelDuringAdmit(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once, relOnce sync.Once
	unblock := func() { relOnce.Do(func() { close(release) }) }
	withSteps(t, map[string]Step{"zz-block": func(context.Context, View) error {
		once.Do(func() { close(entered) })
		<-release
		return nil
	}})
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2))})
	t.Cleanup(unblock) // a failed assert must not leave the tick blocked, or Close would hang
	id := queueAttempt(t, st, "pipe", "proj", "a")
	e, err := d.enqueue(testQueued(id))
	if err != nil {
		t.Fatal(err)
	}
	ticked := make(chan error, 1)
	go func() { ticked <- d.Tick(context.Background()) }() // another goroutine admits the job
	<-entered
	type result struct {
		a   Admitted
		err error
	}
	got := make(chan result, 1)
	go func() { a, err := d.drop(e, context.Canceled); got <- result{a, err} }() // the caller gives up meanwhile
	select {
	case r := <-got:
		t.Fatalf("drop returned while the commit was in flight: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	r := <-got
	if r.err != nil || r.a.LeaseID != "lease-"+id {
		t.Fatalf("the committed lease was not handed back: %+v", r)
	}
	if err = <-ticked; err != nil {
		t.Fatal(err)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), id); state != "leased" {
		t.Fatalf("attempt is %s", state)
	}
	// and a caller that gives up before any claim leaves nothing to admit
	id2 := queueAttempt(t, st, "pipe", "proj", "b")
	e2, _ := d.enqueue(testQueued(id2))
	if _, err = d.drop(e2, context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("drop before a claim: %v", err)
	}
	if err = d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), id2); state != "queued" {
		t.Fatalf("an abandoned job was admitted: %s", state)
	}
}

// TestDispatchDeadlineDuringTick: time that passes inside the tick (the world
// build, an earlier admission) counts: a job past max_queue when its turn comes
// ends with E693; it is not admitted late.
func TestDispatchDeadlineDuringTick(t *testing.T) {
	setup := func(t *testing.T, clock *fakeClock, slowWorld bool) (*Dispatcher, *entry, *entry) {
		st := openStore(t)
		live := liveWorld(st, func(p *sched.Policy) { p.Overrides.MaxQueueMs = 1500 }, testRunner("local", "local", 4))
		world := live
		if slowWorld {
			world = func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
				clock.Advance(2 * time.Second)
				return live(ctx, p)
			}
		}
		d := newDispatcher(t, Config{Store: st, World: world, Clock: clock.Now})
		first, _ := d.enqueue(testQueued(queueAttempt(t, st, "pipe", "proj", "first")))
		second, _ := d.enqueue(testQueued(queueAttempt(t, st, "pipe", "proj", "second")))
		return d, first, second
	}
	timedOut := func(e *entry) bool {
		var de *Error
		return resolved(e) && errors.As(e.err, &de) && de.Code == CodeQueueTimeout
	}
	t.Run("world build", func(t *testing.T) {
		d, first, second := setup(t, newClock(), true)
		if err := d.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !timedOut(first) || !timedOut(second) {
			t.Fatalf("a slow world build: first=%v second=%v", first.err, second.err)
		}
	})
	t.Run("earlier admission", func(t *testing.T) {
		clock := newClock()
		withSteps(t, map[string]Step{"zz-slow": func(context.Context, View) error { clock.Advance(2 * time.Second); return nil }})
		d, first, second := setup(t, clock, false)
		if err := d.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !resolved(first) || first.err != nil {
			t.Fatalf("first job: %v", first.err)
		}
		if !timedOut(second) {
			t.Fatalf("second job after a slow admission: resolved=%v err=%v", resolved(second), second.err)
		}
	})
}

// TestDispatchConflictEndsDeadAttempt: an attempt cancelled elsewhere does not
// conflict for ever; the job ends with an error.
func TestDispatchConflictEndsDeadAttempt(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2))})
	id := queueAttempt(t, st, "pipe", "proj", "a")
	e, _ := d.enqueue(testQueued(id))
	if err := st.Transition(context.Background(), id, "queued", "cancelled", store.TransitionDetail{Epoch: 0, FailureClass: "cancelled"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(e) || e.err == nil {
		t.Fatalf("a cancelled attempt must end the job: resolved=%v err=%v", resolved(e), e.err)
	}
}

// TestDispatchClose: Close ends waiting callers, refuses new work and waits for
// a delivery that was already registered.
func TestDispatchClose(t *testing.T) {
	st := openStore(t)
	started, finish := make(chan struct{}), make(chan struct{})
	node := testRunner("node", "lan", 1)
	d, err := New(Config{Store: st, Host: "host", World: liveWorld(st, nil, node), Poll: time.Millisecond,
		Transport: fakeTransport{run: func(Delivery) error { close(started); <-finish; return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	first := queueAttempt(t, st, "pipe", "proj", "first")
	if _, err = d.Submit(context.Background(), testQueued(first)); err != nil {
		t.Fatal(err)
	}
	<-started
	waiter := queueAttempt(t, st, "pipe", "proj", "waiter") // the node's only slot is taken
	waiting := make(chan error, 1)
	go func() { _, e := d.Submit(context.Background(), testQueued(waiter)); waiting <- e }()
	closed := make(chan struct{})
	go func() { d.Close(); close(closed) }()
	if e := <-waiting; !errors.Is(e, errClosed) {
		t.Fatalf("waiter after Close: %v", e)
	}
	select {
	case <-closed:
		t.Fatal("Close returned while a delivery was running")
	case <-time.After(50 * time.Millisecond):
	}
	close(finish)
	<-closed
	if _, err = d.Submit(context.Background(), testQueued(queueAttempt(t, st, "pipe", "proj", "late"))); !errors.Is(err, errClosed) {
		t.Fatalf("Submit after Close: %v", err)
	}
	if err = d.Tick(context.Background()); !errors.Is(err, errClosed) {
		t.Fatalf("Tick after Close: %v", err)
	}
}

// TestDispatchDeadlineBeforeAdmission: a job past max_queue ends with E693 even
// when capacity freed since the last tick; it does not run late.
func TestDispatchDeadlineBeforeAdmission(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	sink := &recSink{}
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 1)), Clock: clock.Now, Sink: sink})
	busy := queueAttempt(t, st, "pipe", "proj", "busy")
	first, _ := d.enqueue(testQueued(busy))
	if err := d.Tick(context.Background()); err != nil || !resolved(first) {
		t.Fatal(err)
	}
	j := testQueued(queueAttempt(t, st, "pipe", "proj", "late"))
	j.Foreground = true
	e, _ := d.enqueue(j)
	if err := d.Tick(context.Background()); err != nil || resolved(e) {
		t.Fatalf("job should wait: %v", err)
	}
	clock.Advance(11 * time.Minute)
	pass(t, st, busy) // the slot frees before the next tick
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var de *Error
	if !resolved(e) || !errors.As(e.err, &de) || de.Code != CodeQueueTimeout {
		t.Fatalf("a job past its deadline: resolved=%v err=%v", resolved(e), e.err)
	}
	if snap, _ := st.WorldSnapshot(context.Background()); len(snap.Active) != 0 {
		t.Fatalf("the late job got a lease: %+v", snap.Active)
	}
	last := sink.all()
	if got := last[len(last)-1]; got.Outcome != OutcomeTimeout || got.Reason != model.QueueTimeout {
		t.Fatalf("decision: %+v", got)
	}
}
