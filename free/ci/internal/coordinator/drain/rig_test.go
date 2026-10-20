package drain

// Purpose: the shared rig of the drain tests: a fake clock with timers, an in-memory Leases that keeps an
//          ordered event log, a recording Canceller and Intake, and the goroutine-leak check.
// Inputs:  none. Outputs: helpers only.
// Constraints: no real sleeping beyond a 1 ms poll while waiting for the drain to block on the fake clock.

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"testing"
	"time"
)

type timer struct {
	due time.Time
	ch  chan time.Time
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []timer
}

func newClock() *fakeClock { return &fakeClock{now: time.Unix(1_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.timers = append(c.timers, timer{c.now.Add(d), ch})
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	keep := c.timers[:0]
	for _, t := range c.timers {
		if !t.due.After(c.now) {
			t.ch <- c.now
		} else {
			keep = append(keep, t)
		}
	}
	c.timers = keep
}

func (c *fakeClock) waiters() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// settleOn waits until every goroutine of the drain is blocked on the fake clock (n timers) or done closes.
func (c *fakeClock) settleOn(t *testing.T, n int, done <-chan struct{}) bool {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(time.Millisecond) {
		select {
		case <-done:
			return false
		default:
		}
		if c.waiters() >= n {
			return true
		}
	}
	t.Fatal("the drain did not block on the clock")
	return false
}

// run advances the clock by step each time the drain is blocked, until it returns.
func (c *fakeClock) run(t *testing.T, n int, step time.Duration, done <-chan struct{}, each func()) {
	t.Helper()
	for c.settleOn(t, n, done) {
		if each != nil {
			each()
		}
		c.Advance(step)
	}
}

type event struct {
	Seq  int
	At   time.Time
	What string
}

type fakeWorld struct {
	mu       sync.Mutex
	clock    *fakeClock
	events   []event
	leases   map[string]*Lease
	state    map[string]string // running | finished | requeued | needs-operator
	renew    bool              // Get extends the deadline every call (an agent that ignores the cancel)
	stopped  int
	onCancel func(leaseID string)
}

func newWorld(c *fakeClock) *fakeWorld {
	return &fakeWorld{clock: c, leases: map[string]*Lease{}, state: map[string]string{}}
}

func (w *fakeWorld) log(format string, a ...any) {
	w.events = append(w.events, event{len(w.events), w.clock.Now(), fmt.Sprintf(format, a...)})
}

func (w *fakeWorld) add(id, kind string, idempotent bool, ttl time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.leases[id] = &Lease{ID: id, AttemptID: "att-" + id, NodeID: "node-" + id, Kind: kind, Idempotent: idempotent, Epoch: 1, ExpiresAt: w.clock.Now().Add(ttl)}
	w.state[id] = "running"
}

func (w *fakeWorld) finish(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state[id] = "finished"
	w.log("finish %s", id)
}

func (w *fakeWorld) names() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, len(w.events))
	for i, e := range w.events {
		out[i] = e.What
	}
	return out
}

func (w *fakeWorld) find(what string) (event, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range w.events {
		if e.What == what {
			return e, true
		}
	}
	return event{}, false
}

func (w *fakeWorld) setOnCancel(f func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onCancel = f
}

func (w *fakeWorld) stops() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stopped
}

// Stop implements Intake.
func (w *fakeWorld) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped++
	w.log("intake.stop")
}

// Cancel implements Canceller.
func (w *fakeWorld) Cancel(nodeID, leaseID, reason string) error {
	w.mu.Lock()
	w.log("cancel %s node=%s", leaseID, nodeID)
	cb := w.onCancel
	w.mu.Unlock()
	if cb != nil {
		cb(leaseID)
	}
	return nil
}

func (w *fakeWorld) Running(context.Context) ([]Lease, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []Lease
	for id, l := range w.leases {
		if w.state[id] == "running" {
			out = append(out, *l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (w *fakeWorld) Get(_ context.Context, id string) (Lease, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state[id] != "running" {
		return Lease{}, false, nil
	}
	if w.renew {
		w.leases[id].ExpiresAt = w.clock.Now().Add(30 * time.Second)
	}
	return *w.leases[id], true, nil
}

func (w *fakeWorld) HandBack(_ context.Context, l Lease, expired bool, _ string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state[l.ID] = "requeued"
	w.log("handback %s expired=%t", l.ID, expired)
	return nil
}

func (w *fakeWorld) NeedsOperator(_ context.Context, l Lease, _ string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state[l.ID] = "needs-operator"
	w.log("needs-operator %s", l.ID)
	return nil
}

func (w *fakeWorld) indexOf(t *testing.T, what string) int {
	t.Helper()
	for i, n := range w.names() {
		if n == what {
			return i
		}
	}
	t.Fatalf("event %q not logged; log: %v", what, w.names())
	return -1
}

func (w *fakeWorld) never(t *testing.T, what string) {
	t.Helper()
	for _, n := range w.names() {
		if n == what {
			t.Fatalf("event %q must not happen; log: %v", what, w.names())
		}
	}
}

// noLeak fails when goroutines outlive the test body; it dumps every stack on failure.
func noLeak(t *testing.T) {
	t.Helper()
	_, warm := NotifyTerm() // the runtime starts its signal goroutine once and keeps it
	warm()
	base := runtime.NumGoroutine()
	t.Cleanup(func() {
		var n int
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
			if n = runtime.NumGoroutine(); n <= base {
				return
			}
		}
		var buf bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&buf, 1)
		t.Errorf("goroutine leak: %d now, %d before\n%s", n, base, buf.String())
	})
}
