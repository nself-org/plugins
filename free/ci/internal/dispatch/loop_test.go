package dispatch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// enqueueAt queues attempts on d, one second apart, and returns their entries.
func enqueueAt(t *testing.T, d *Dispatcher, clock *fakeClock, ids ...string) []*entry {
	t.Helper()
	var out []*entry
	for _, id := range ids {
		e, err := d.enqueue(testQueued(id))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
		clock.Advance(time.Second)
	}
	return out
}

// TestDispatchTick: Place gets the Orderer's order, admission keeps it, one
// tick never admits more jobs to a runner than its free slots, and finished
// work frees the next job.
func TestDispatchTick(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	sink := &recSink{}
	local := testRunner("local", "local", 2)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, local), Clock: clock.Now, Sink: sink})
	ids := []string{queueAttempt(t, st, "pipe", "proj", "a"), queueAttempt(t, st, "pipe", "proj", "b"), queueAttempt(t, st, "pipe", "proj", "c")}
	es := enqueueAt(t, d, clock, ids...)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(es[0]) || !resolved(es[1]) || resolved(es[2]) {
		t.Fatalf("2 free slots: resolved = %v %v %v", resolved(es[0]), resolved(es[1]), resolved(es[2]))
	}
	if es[0].res.RunnerID != "local" || !es[0].res.Local || es[0].res.LeaseID != "lease-"+ids[0] {
		t.Fatalf("first admission: %+v", es[0].res)
	}
	got := sink.all()
	if len(got) != 3 || got[0].Outcome != OutcomeAdmitted || got[1].Outcome != OutcomeAdmitted || got[2].Outcome != OutcomeWaiting || got[0].Job.Key != ids[0] || got[1].Job.Key != ids[1] {
		t.Fatalf("decisions: %+v", got)
	}
	for _, g := range got {
		if g.WorldDigest == "" {
			t.Fatal("decision without a world digest")
		}
	}
	if reason, _, ok, _ := st.Refusal(context.Background(), ids[2]); !ok || reason != string(model.CapacityBusy) {
		t.Fatalf("waiting job reason = %q %v", reason, ok)
	}
	if err := d.Tick(context.Background()); err != nil || resolved(es[2]) {
		t.Fatalf("a second tick with no free slot admitted: %v", err)
	}
	pass(t, st, ids[0])
	if err := d.Tick(context.Background()); err != nil || !resolved(es[2]) || es[2].err != nil {
		t.Fatalf("freed slot not used: %v %v", err, es[2].err)
	}
}

type reverse struct{}

func (reverse) Order(q []Queued) []Queued {
	out := FIFO{}.Order(q)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// TestDispatchOrderer: a replaced Orderer decides who gets the slot.
func TestDispatchOrderer(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 1)), Clock: clock.Now, Orderer: reverse{}})
	es := enqueueAt(t, d, clock, queueAttempt(t, st, "pipe", "proj", "old"), queueAttempt(t, st, "pipe", "proj", "new"))
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if resolved(es[0]) || !resolved(es[1]) {
		t.Fatalf("reverse order: old=%v new=%v", resolved(es[0]), resolved(es[1]))
	}
	q := []Queued{{Seq: 2, EnqueuedAt: clock.Now()}, {Seq: 1, EnqueuedAt: clock.Now()}}
	if got := (FIFO{}).Order(q); got[0].Seq != 1 || q[0].Seq != 2 {
		t.Fatalf("FIFO ties by arrival and leaves its input alone: %+v", got)
	}
}

// TestDispatchStaleWorld: a world that lags the store cannot oversubscribe a
// runner, because the admission transaction re-reads counters and reservations.
func TestDispatchStaleWorld(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	local := testRunner("local", "local", 1)
	stale := func(context.Context, sched.Pipeline) (sched.World, string, error) {
		return sched.World{Runners: []sched.Runner{local}, Policy: sched.Policy{Preset: "balanced"}, Costs: map[string]sched.Cost{}, Previous: map[string]string{}}, "stale", nil
	}
	sink := &recSink{}
	d := newDispatcher(t, Config{Store: st, World: stale, Clock: clock.Now, Sink: sink})
	es := enqueueAt(t, d, clock, queueAttempt(t, st, "pipe", "proj", "a"), queueAttempt(t, st, "pipe", "proj", "b"))
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(es[0]) || resolved(es[1]) {
		t.Fatalf("one slot: %v %v", resolved(es[0]), resolved(es[1]))
	}
	enqueueAt(t, d, clock, queueAttempt(t, st, "pipe", "proj", "c"))
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := sink.all()
	refused := last[len(last)-2]
	if refused.Outcome != OutcomeRefused || refused.Reason != model.LimitRunner {
		t.Fatalf("stale placement: %+v", refused)
	}
	snap, _ := st.WorldSnapshot(context.Background())
	if snap.Reservations["local"].Slots != 1 {
		t.Fatalf("local runner holds %d leases for 1 slot", snap.Reservations["local"].Slots)
	}
}

// TestDispatchLimitQueues: a policy limit queues the job with limit.<level>
// recorded, and lets it go once the count drops.
func TestDispatchLimitQueues(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	policy := func(p *sched.Policy) { p.Limits.Global = 1 }
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, policy, testRunner("local", "local", 4)), Clock: clock.Now})
	ids := []string{queueAttempt(t, st, "pipe", "proj", "a"), queueAttempt(t, st, "pipe", "proj", "b")}
	es := enqueueAt(t, d, clock, ids...)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(es[0]) || resolved(es[1]) {
		t.Fatal("global limit 1 must admit one job")
	}
	if reason, _, ok, _ := st.Refusal(context.Background(), ids[1]); !ok || reason != string(model.LimitGlobal) {
		t.Fatalf("recorded reason = %q", reason)
	}
	pass(t, st, ids[0])
	if err := d.Tick(context.Background()); err != nil || !resolved(es[1]) {
		t.Fatalf("limit released: %v", err)
	}
}

// TestDispatchQueueTimeout: a foreground job past max_queue ends with E693; a
// background job without max_queue waits.
func TestDispatchQueueTimeout(t *testing.T) {
	for _, c := range []struct {
		name       string
		foreground bool
		maxQueue   int64
		wait       time.Duration
		wantCode   string
	}{
		{"foreground default 10m", true, 0, 11 * time.Minute, "E693"},
		{"foreground before default", true, 0, 9 * time.Minute, ""},
		{"policy max_queue", true, 1000, 2 * time.Second, "E693"},
		{"background no limit", false, 0, 48 * time.Hour, ""},
		{"background with max_queue", false, 5000, time.Minute, "E693"},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := openStore(t)
			clock := newClock()
			busy := queueAttempt(t, st, "pipe", "proj", "busy")
			policy := func(p *sched.Policy) { p.Overrides.MaxQueueMs = c.maxQueue }
			d := newDispatcher(t, Config{Store: st, World: liveWorld(st, policy, testRunner("local", "local", 1)), Clock: clock.Now})
			first, _ := d.enqueue(testQueued(busy))
			if err := d.Tick(context.Background()); err != nil || !resolved(first) {
				t.Fatalf("occupant: %v", err)
			}
			j := testQueued(queueAttempt(t, st, "pipe", "proj", "waiter"))
			j.Foreground = c.foreground
			e, err := d.enqueue(j)
			if err != nil {
				t.Fatal(err)
			}
			if err = d.Tick(context.Background()); err != nil || resolved(e) {
				t.Fatalf("waiting job: %v", err)
			}
			clock.Advance(c.wait)
			if err = d.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			var de *Error
			switch {
			case c.wantCode == "" && resolved(e):
				t.Fatalf("resolved early: %v", e.err)
			case c.wantCode != "" && (!resolved(e) || !errors.As(e.err, &de) || de.Code != c.wantCode || !strings.Contains(de.Message, "queue.timeout")):
				t.Fatalf("want %s, got resolved=%v err=%v", c.wantCode, resolved(e), e.err)
			}
		})
	}
}

// TestDispatchUnplaceable: a foreground job no runner can ever take fails at
// once with E618 and one reason line; a background one stays queued.
func TestDispatchUnplaceable(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 2)), Clock: clock.Now})
	needTool := func(id string, foreground bool) *entry {
		j := testQueued(id)
		j.Sched.Requirements.Tools = []string{"no-such-tool"}
		j.Foreground = foreground
		e, err := d.enqueue(j)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	fg := needTool(queueAttempt(t, st, "pipe", "proj", "fg"), true)
	bg := needTool(queueAttempt(t, st, "pipe", "proj", "bg"), false)
	if err := d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	var de *Error
	if !resolved(fg) || !errors.As(fg.err, &de) || de.Code != "E618" || !strings.Contains(de.Message, "tools.missing") || strings.Contains(de.Message, "\n") {
		t.Fatalf("foreground: resolved=%v err=%v", resolved(fg), fg.err)
	}
	if resolved(bg) {
		t.Fatal("a background job must wait, not fail")
	}
	if reason, _, ok, _ := st.Refusal(context.Background(), "pipe.bg.2"); ok && reason != "tools.missing" {
		t.Fatalf("reason %q", reason)
	}
}

// TestDispatchSubmit: Submit blocks until admitted; waiters share capacity and
// finish in turn; a cancelled caller leaves the queue.
func TestDispatchSubmit(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("local", "local", 1)), Poll: time.Millisecond})
	ctx := context.Background()
	ids := []string{queueAttempt(t, st, "pipe", "proj", "a"), queueAttempt(t, st, "pipe", "proj", "b"), queueAttempt(t, st, "pipe", "proj", "c")}
	done := make(chan string, len(ids))
	for _, id := range ids {
		go func() {
			a, err := d.Submit(ctx, testQueued(id))
			if err != nil {
				done <- "error: " + err.Error()
				return
			}
			pass(t, st, a.AttemptID)
			done <- a.AttemptID
		}()
	}
	seen := map[string]bool{}
	for range ids {
		select {
		case id := <-done:
			seen[id] = true
		case <-time.After(30 * time.Second):
			t.Fatalf("waiters stuck: %v", seen)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("attempts finished: %v", seen)
	}
	cctx, cancel := context.WithCancel(ctx)
	stuck := queueAttempt(t, st, "pipe", "proj", "never")
	blocker := queueAttempt(t, st, "pipe", "proj", "blocker")
	if _, err := d.Submit(ctx, testQueued(blocker)); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := d.Submit(cctx, testQueued(stuck)); result <- err }()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter: %v", err)
	}
	if _, err := d.Submit(ctx, Job{}); err == nil {
		t.Fatal("empty job accepted")
	}
	d.mu.Lock()
	n := len(d.queue)
	d.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d entries left in the queue", n)
	}
}

// TestDispatchOnStart: hooks run when a dispatcher is constructed; a failing
// hook fails New.
func TestDispatchOnStart(t *testing.T) {
	onStartMu.Lock()
	saved := onStart
	onStart = nil
	onStartMu.Unlock()
	t.Cleanup(func() { onStartMu.Lock(); onStart = saved; onStartMu.Unlock() })
	st := openStore(t)
	ran := 0
	OnStart(func(*Dispatcher) error { ran++; return nil })
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil)})
	if ran != 1 || d == nil {
		t.Fatalf("OnStart ran %d times", ran)
	}
	OnStart(func(*Dispatcher) error { return errors.New("sweep failed") })
	if _, err := New(Config{Store: st, World: liveWorld(st, nil), Host: "h"}); err == nil || ran != 2 {
		t.Fatalf("failing hook: %v (ran %d)", err, ran)
	}
	if _, err := New(Config{}); err == nil {
		t.Fatal("an empty config built a dispatcher")
	}
}

// TestDispatchWorldError: a foreground job fails with the world error instead of hanging.
func TestDispatchWorldError(t *testing.T) {
	st := openStore(t)
	boom := errors.New("E699: world snapshot unavailable")
	d := newDispatcher(t, Config{Store: st, World: func(context.Context, sched.Pipeline) (sched.World, string, error) { return sched.World{}, "", boom }})
	j := testQueued(queueAttempt(t, st, "pipe", "proj", "a"))
	j.Foreground = true
	e, _ := d.enqueue(j)
	if err := d.Tick(context.Background()); !errors.Is(err, boom) || !resolved(e) || !errors.Is(e.err, boom) {
		t.Fatalf("world error: tick=%v job=%v", err, e.err)
	}
}
