package dispatch

// Purpose: the dispatcher (Epic D7): a queue of attempts waiting for a
// placement and the tick that places and admits them. A tick runs when a job
// is enqueued, when a waiter's poll fires (attempt end and lease expiry free
// capacity without a signal), and every Poll interval (one second in TEAM mode).
// Inputs:  Config (store, world builder, orderer, sink, transport, clock) and
// Submit calls, one per attempt, each blocking until its attempt is admitted.
// Outputs: Admitted attempts; E618 for a foreground job no runner can take;
// E693 for a foreground job that waited past max_queue.
// Constraints: waiters tick cooperatively under one lock, so no goroutine
// outlives a Submit except lease deliveries (Close waits for them); Tick is
// safe to call directly from deterministic tests with an injected Clock.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// defaultForegroundQueueMs is the foreground max_queue when policy sets none (R-DX-06).
const defaultForegroundQueueMs = 10 * 60 * 1000

// Job is one attempt waiting for a placement.
type Job struct {
	AttemptID string
	// Sched is the placement input; Sched.Key must be unique among queued jobs
	// and Sched.Pipeline.ID names the pipeline whose world and policy apply.
	Sched sched.Job
	// Model is the job definition a remote driver receives.
	Model model.Job
	// Revision is the commit a remote runner checks out.
	Revision   string
	Foreground bool
	// LocalCPU and LocalMemMB are the host capacity a local placement reserves
	// against (0 = from the local runner's capability).
	LocalCPU, LocalMemMB int
}

// Config wires a dispatcher.
type Config struct {
	Store *store.Store
	// Host scopes the local runner's reservations (X10).
	Host string
	// World builds the immutable placement input for one pipeline (P7-SCHED-18).
	World     func(context.Context, sched.Pipeline) (sched.World, string, error)
	Orderer   Orderer
	Sink      DecisionSink
	Transport Transport
	Clock     func() time.Time
	// Poll is how often a waiter re-ticks; Team selects the one-second TEAM tick.
	Poll time.Duration
	Team bool
	// Incarnation is the coordinator incarnation fenced leases carry (default 1).
	Incarnation func() int64
	// LeaseTTL is the time a remote lease lives without a heartbeat (default 45s,
	// P7-NODE-09); a local lease uses the store default.
	LeaseTTL time.Duration
	// ProviderLimit is the per-provider concurrency limit (0 = none).
	ProviderLimit func(provider string) int
	// DeliveryError observes a failed carrier run (the attempt is already marked lost).
	DeliveryError func(Admitted, error)
}

type entry struct {
	queued   Queued
	done     chan struct{}
	res      Admitted
	err      error
	finished bool
	reason   model.PlacementReason
	fails    int // consecutive store errors while admitting
}

// Dispatcher places and admits queued attempts.
type Dispatcher struct {
	cfg   Config
	life  context.Context
	stop  context.CancelFunc
	wg    sync.WaitGroup
	tickM sync.Mutex

	mu       sync.Mutex
	queue    []*entry
	seq      uint64
	ticked   uint64
	lastTick time.Time
}

var (
	onStartMu sync.Mutex
	onStart   []func(*Dispatcher) error
)

// OnStart registers a hook that runs when a dispatcher is constructed (burst sweep, D12).
func OnStart(f func(*Dispatcher) error) {
	onStartMu.Lock()
	defer onStartMu.Unlock()
	onStart = append(onStart, f)
}

// New constructs a dispatcher and runs the OnStart hooks; a failing hook fails New.
func New(cfg Config) (*Dispatcher, error) {
	if cfg.Store == nil || cfg.World == nil || cfg.Host == "" {
		return nil, fail(CodeUnplaceable, "a dispatcher needs a store, a world builder and a host")
	}
	if cfg.Orderer == nil {
		cfg.Orderer = FIFO{}
	}
	if cfg.Sink == nil {
		cfg.Sink = noSink{}
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 45 * time.Second
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 50 * time.Millisecond
		if cfg.Team {
			cfg.Poll = time.Second
		}
	}
	d := &Dispatcher{cfg: cfg}
	d.life, d.stop = context.WithCancel(context.Background())
	onStartMu.Lock()
	hooks := append([]func(*Dispatcher) error(nil), onStart...)
	onStartMu.Unlock()
	for _, h := range hooks {
		if err := h(d); err != nil {
			d.Close()
			return nil, err
		}
	}
	return d, nil
}

// Close stops deliveries and waits for them.
func (d *Dispatcher) Close() {
	d.stop()
	d.wg.Wait()
}

// Submit queues the job and blocks until it holds a lease, ctx is done
// (ctx.Err()), or the dispatcher refuses it (E618, E693, or a store error).
func (d *Dispatcher) Submit(ctx context.Context, j Job) (Admitted, error) {
	e, err := d.enqueue(j)
	if err != nil {
		return Admitted{}, err
	}
	for {
		if d.due() {
			_ = d.Tick(ctx) // a failed tick leaves the job queued; foreground world errors finish it
		}
		select {
		case <-e.done:
			return e.res, e.err
		case <-ctx.Done():
			return d.drop(e, ctx.Err())
		case <-time.After(d.cfg.Poll):
		}
	}
}

func (d *Dispatcher) enqueue(j Job) (*entry, error) {
	if j.AttemptID == "" || j.Sched.Key == "" {
		return nil, fail(CodeUnplaceable, "a queued job needs an attempt and a key")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.queue {
		if !e.finished && e.queued.Job.Sched.Key == j.Sched.Key {
			return nil, fail(CodeUnplaceable, "job key %q is already queued", j.Sched.Key)
		}
	}
	d.seq++
	e := &entry{queued: Queued{Job: j, EnqueuedAt: d.cfg.Clock(), Seq: d.seq}, done: make(chan struct{})}
	d.queue = append(d.queue, e)
	return e, nil
}

// due reports whether a waiter should tick: something was enqueued since the
// last tick, or Poll has passed.
func (d *Dispatcher) due() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.seq != d.ticked || d.cfg.Clock().Sub(d.lastTick) >= d.cfg.Poll
}

// drop removes an unfinished entry (its caller gave up). An entry a tick
// admitted just before the caller gave up keeps its result: the lease exists
// and the caller must see it to cancel it.
func (d *Dispatcher) drop(e *entry, cause error) (Admitted, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !e.finished {
		e.finished, e.err = true, cause
		close(e.done)
	}
	d.compact()
	return e.res, e.err
}

// finish resolves the entry with sequence seq once.
func (d *Dispatcher) finish(seq uint64, a Admitted, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range d.queue {
		if e.queued.Seq == seq && !e.finished {
			e.finished, e.res, e.err = true, a, err
			close(e.done)
		}
	}
	d.compact()
}

func (d *Dispatcher) compact() {
	live := d.queue[:0]
	for _, e := range d.queue {
		if !e.finished {
			live = append(live, e)
		}
	}
	d.queue = live
}

// pending snapshots the unfinished entries.
func (d *Dispatcher) pending(now time.Time) []Queued {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ticked, d.lastTick = d.seq, now
	out := make([]Queued, 0, len(d.queue))
	for _, e := range d.queue {
		if !e.finished {
			out = append(out, e.queued)
		}
	}
	return out
}

// Tick places and admits every queued job once, in the Orderer's order.
//
// A tick works for every queued job, so it does not stop when the caller that
// happened to run it gives up: that caller's cancellation must not fail
// another caller's job.
func (d *Dispatcher) Tick(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx = context.WithoutCancel(ctx)
	d.tickM.Lock()
	defer d.tickM.Unlock()
	now := d.cfg.Clock()
	queued := d.pending(now)
	if len(queued) == 0 {
		return nil
	}
	ordered := d.cfg.Orderer.Order(queued)
	if len(ordered) != len(queued) {
		return errors.New("dispatch: the orderer must return exactly the queued jobs")
	}
	var groups [][]Queued
	index := map[string]int{}
	for _, q := range ordered {
		id := q.Job.Sched.Pipeline.ID
		i, ok := index[id]
		if !ok {
			i = len(groups)
			index[id] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], q)
	}
	var first error
	for _, g := range groups {
		if err := d.tickGroup(ctx, g, now); err != nil && first == nil {
			first = err
		}
	}
	return first
}
