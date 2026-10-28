//go:build scenarios

package scenarios

// Purpose: the rig of scenario 10 (Epic acceptance 4, capacity saturation) and the check functions
// the scenario and its planted regressions share. A flood of jobs from two projects meets a few
// slots through a real dispatcher over a real store; the test, not a timer, decides when work ends.
// Inputs:  runners with known slots, jobs per project and band.
// Outputs: an admission log in the order the dispatcher admitted, and violation lists.
// Constraints: no sleeps. A closed gate makes the world builder fail, which leaves every job queued
//          (the dispatcher keeps a non-foreground job whose world cannot be built), so the whole
//          flood is queued before the first placement and the fair order is deterministic.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/planview"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// s10Runner is a runner of project alpha and beta that background work may use (plugged in, idle).
func s10Runner(id, loc string, slots int) sched.Runner {
	r := runner(id, loc, "linux/amd64", slots)
	r.Capability.Lifecycle.AuthorizedProjects = []string{"alpha", "beta"}
	r.Capability.Availability.PluggedIn.Value = ptr(true)
	r.Capability.Availability.Interactive.Value = ptr("idle")
	return r
}

// s10Register enrols a LAN runner as an online node authorized for alpha and beta (the rig's
// registerNode authorizes only "project"); liveWorld's own registration then finds it present.
func s10Register(t testing.TB, st *store.Store, r sched.Runner) {
	t.Helper()
	c, err := planview.LocalCapability(planview.LocalFacts{Project: "alpha", Platform: "linux/amd64", CPUs: 8, MemMB: 8192, Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = r.ID, r.ID, "ssh"
	c.Lifecycle.AuthorizedProjects = []string{"alpha", "beta"}
	doc, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodePut(context.Background(), store.NodeRecord{ID: r.ID, Name: r.ID, State: "online", Lifecycle: []byte("{}"), Trust: []byte("{}"), DeployHost: []byte("{}"), Capability: doc, ObservedAt: time.Now().UnixNano()}, "scenarios"); err != nil {
		t.Fatal(err)
	}
}

// s10Job is a queued attempt of project proj in band band ("" = normal).
func s10Job(t testing.TB, st *store.Store, proj, band string) (string, dispatch.Job) {
	attempt := queueAttempt(t, st, "pipe-"+proj, proj, "job")
	j := dispatch.Job{AttemptID: attempt, Sched: job(attempt, "linux/amd64")}
	j.Sched.Project, j.Sched.Pipeline.ID, j.Sched.Pipeline.Band = proj, "pipe-"+proj, band
	return attempt, j
}

// s10Event is one admission or unplaceable decision.
type s10Event struct{ Key, Project, Band, Runner, Outcome string }

// s10Sink logs admissions in tick order and runs check after each one, inside the tick.
type s10Sink struct {
	mu    sync.Mutex
	log   []s10Event
	viol  []string
	ch    chan struct{}
	check func(context.Context) []string
}

func (s *s10Sink) Record(ctx context.Context, d dispatch.Decision) {
	if d.Outcome != dispatch.OutcomeAdmitted && d.Outcome != dispatch.OutcomeUnplaceable {
		return
	}
	ev := s10Event{Key: d.Job.Key, Project: d.Job.Project, Band: d.Job.Pipeline.Band, Outcome: d.Outcome}
	if d.Placement != nil {
		ev.Runner = d.Placement.RunnerID
	}
	var v []string
	if d.Outcome == dispatch.OutcomeAdmitted && s.check != nil {
		v = s.check(ctx)
	}
	s.mu.Lock()
	s.log, s.viol = append(s.log, ev), append(s.viol, v...)
	s.mu.Unlock()
	select {
	case s.ch <- struct{}{}:
	default:
	}
}

func (s *s10Sink) events(outcome string) []s10Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []s10Event
	for _, e := range s.log {
		if e.Outcome == outcome {
			out = append(out, e)
		}
	}
	return out
}

func (s *s10Sink) violations() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.viol...)
}

// s10Orderer wraps the fair queue and remembers the queue length of the last tick.
type s10Orderer struct {
	inner dispatch.Orderer
	mu    sync.Mutex
	last  int
	most  int // the longest queue any tick ordered
	ch    chan struct{}
}

func (o *s10Orderer) Order(q []dispatch.Queued) []dispatch.Queued {
	o.mu.Lock()
	o.last = len(q)
	o.most = max(o.most, len(q))
	o.mu.Unlock()
	select {
	case o.ch <- struct{}{}:
	default:
	}
	return o.inner.Order(q)
}

func (o *s10Orderer) mostLen() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.most
}

func (o *s10Orderer) lastLen() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.last
}

// s10Rig is a dispatcher over a store with a gated world.
type s10Rig struct {
	t         testing.TB
	st        *store.Store
	clk       *clock
	d         *dispatch.Dispatcher
	ord       *s10Orderer
	sink      *s10Sink
	open      atomic.Bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	attempts  []string
	errs      chan error
	trueSlots map[string]int
}

// newS10Rig builds the rig; the gate starts closed. world (nil: the honest live world over
// runners) is what the gate guards.
func newS10Rig(t testing.TB, runners []sched.Runner, world func(*store.Store) func(context.Context, sched.Pipeline) (sched.World, string, error), queue func(context.Context) (dispatch.QueueLimits, error)) *s10Rig {
	st := openStore(t)
	r := &s10Rig{t: t, st: st, clk: newClock(), errs: make(chan error, 64), trueSlots: map[string]int{}}
	r.ctx, r.cancel = context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(r.cancel)
	for _, ru := range runners {
		if ru.Location == "lan" {
			s10Register(t, st, ru)
		}
		r.trueSlots[ru.ID] = *ru.Capability.Availability.Slots.Value
	}
	live := liveWorld(t, st, nil, runners...)
	if world != nil {
		live = world(st)
	}
	gated := func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
		if !r.open.Load() {
			return sched.World{}, "", errors.New("gate closed")
		}
		return live(ctx, p)
	}
	r.ord = &s10Orderer{inner: dispatch.NewFair(nil, r.clk.Now).WithCost(func() func(string) int64 { return dispatch.RuntimeCost(context.Background(), st) }), ch: make(chan struct{}, 1)}
	r.sink = &s10Sink{ch: make(chan struct{}, 1), check: func(ctx context.Context) []string {
		return s10CheckCapacity(ctx, st, r.trueSlots, 8)
	}}
	r.d = newDispatcher(t, dispatch.Config{Store: st, World: gated, Poll: time.Millisecond, Transport: &transport{}, Clock: r.clk.Now, Orderer: r.ord, Sink: r.sink, Queue: queue, Slots: func() int { return 6 }})
	t.Cleanup(r.wg.Wait)
	t.Cleanup(r.cancel)
	return r
}

// submit queues one job in a goroutine and returns its attempt id.
func (r *s10Rig) submit(proj, band string) string {
	attempt, j := s10Job(r.t, r.st, proj, band)
	r.attempts = append(r.attempts, attempt)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if _, err := r.d.Submit(r.ctx, j); err != nil {
			r.errs <- fmt.Errorf("%s: %w", attempt, err)
		}
	}()
	return attempt
}

// waitQueued blocks until a tick saw exactly n queued jobs.
func (r *s10Rig) waitQueued(n int) {
	r.t.Helper()
	for r.ord.lastLen() != n {
		select {
		case <-r.ord.ch:
		case <-r.ctx.Done():
			r.t.Fatalf("queue never reached %d jobs (last tick saw %d)", n, r.ord.lastLen())
		}
	}
}

// waitAdmitted blocks until n admissions are logged.
func (r *s10Rig) waitAdmitted(n int) {
	r.t.Helper()
	for len(r.sink.events(dispatch.OutcomeAdmitted)) < n {
		select {
		case <-r.sink.ch:
		case <-r.ctx.Done():
			r.t.Fatalf("only %d of %d admissions", len(r.sink.events(dispatch.OutcomeAdmitted)), n)
		}
	}
}

// release opens the gate and ticks once, synchronously. The clock stays frozen on purpose: the
// waiting Submit callers tick on their own only when the clock moves or a job is enqueued, so
// every later tick is one the test makes and the order of events is deterministic.
func (r *s10Rig) release() {
	r.open.Store(true)
	r.tick()
}

// finish ends attempt id (its capacity frees) and ticks. A tick that a waiting caller starts on its
// own while the attempt walks to passed reads one store snapshot for every pipeline's world
// (D-1412), so no later pipeline can plan on capacity an earlier one did not see.
func (r *s10Rig) finish(id string) {
	pass(r.t, r.st, id)
	r.release()
}

func (r *s10Rig) tick() {
	r.t.Helper()
	if err := r.d.Tick(r.ctx); err != nil {
		r.t.Fatal(err)
	}
}

// s10CheckCapacity returns the breaches of TRUE capacity in the store's view: a runner with more
// active leases, reserved slots or reserved CPU than it really has.
func s10CheckCapacity(ctx context.Context, st *store.Store, slots map[string]int, cpu int64) []string {
	snap, err := st.WorldSnapshot(ctx)
	if err != nil {
		return []string{"snapshot: " + err.Error()}
	}
	var out []string
	active := map[string]int{}
	for _, a := range snap.Active {
		active[a.RunnerID]++
	}
	for id, n := range active {
		if n > slots[id] {
			out = append(out, fmt.Sprintf("runner %s holds %d leases, it has %d slots", id, n, slots[id]))
		}
	}
	for id, rs := range snap.Reservations {
		if int(rs.Slots) > slots[id] || rs.CPU > cpu {
			out = append(out, fmt.Sprintf("runner %s reserved %d slots and %d cpu, capacity %d and %d", id, rs.Slots, rs.CPU, slots[id], cpu))
		}
	}
	return out
}
