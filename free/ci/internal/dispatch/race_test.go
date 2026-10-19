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

// TestDispatchCancelDuringAdmit: a caller that gives up while its lease is being
// committed gets the lease back instead of losing its owner.
func TestDispatchCancelDuringAdmit(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	withSteps(t, map[string]Step{"zz-block": func(context.Context, View) error {
		once.Do(func() { close(entered) })
		<-release
		return nil
	}})
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2)), Poll: time.Millisecond})
	id := queueAttempt(t, st, "pipe", "proj", "a")
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		a   Admitted
		err error
	}
	got := make(chan result, 1)
	go func() { a, err := d.Submit(ctx, testQueued(id)); got <- result{a, err} }()
	<-entered
	cancel()
	select {
	case r := <-got:
		t.Fatalf("Submit returned while the commit was in flight: %+v", r)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	r := <-got
	if r.err != nil || r.a.LeaseID != "lease-"+id {
		t.Fatalf("the committed lease was not handed back: %+v", r)
	}
	if state, _, _ := st.AttemptStateEpoch(context.Background(), id); state != "leased" {
		t.Fatalf("attempt is %s", state)
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
