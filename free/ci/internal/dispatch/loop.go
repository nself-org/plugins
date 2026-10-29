package dispatch

// Purpose: the dispatcher (Epic D7): a queue of attempts waiting for a
// placement and the tick that places and admits them. A tick runs when a job
// is enqueued, when a waiter's poll fires (attempt end and lease expiry free
// capacity without a signal unless the caller calls Wake), and every Poll
// interval (one second in TEAM mode).
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
	// NoDefaultQueueCap withholds the implicit 10 minute queue cap of a Foreground
	// job: nobody waits at a terminal for a scripted or scheduled run. The policy's
	// max_queue still applies (P7-AUTO-29).
	NoDefaultQueueCap bool
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
	World func(context.Context, sched.Pipeline) (sched.World, string, error)
	// Orderer sets the placement order; nil is the Fair orderer (D9).
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
	// TickTimeout bounds one tick (world builds, policy resolution, admissions);
	// the default is two minutes. A tick ignores its caller's cancellation, so
	// this is what keeps a hung dependency from holding the dispatcher for ever.
	TickTimeout time.Duration
	// Queue returns the fairness and queue-depth policy (sched.fairness.starvation_after,
	// sched.queue.max_depth); nil, or an error, means the registered defaults.
	Queue func(context.Context) (QueueLimits, error)
	// Slots is the slot count a new job competes for, used for the expected wait
	// of a refused submission (nil = 1).
	Slots func() int
	// DeliveryError observes a failed carrier run (the attempt is already marked lost).
	DeliveryError func(Admitted, error)
}

// Dispatcher places and admits queued attempts.
type Dispatcher struct {
	cfg   Config
	life  context.Context
	stop  context.CancelFunc
	wg    sync.WaitGroup
	tickM sync.Mutex
	// quiesced is set under tickM by Close: no tick is running.
	quiesced bool

	mu       sync.Mutex
	closed   bool
	queue    []*entry
	seq      uint64
	ticked   uint64
	lastTick time.Time
	wake     chan struct{} // closed by Wake: the waiters tick now
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
	if cfg.Sink == nil {
		cfg.Sink = defaultSink(cfg)
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.TickTimeout <= 0 {
		cfg.TickTimeout = 2 * time.Minute
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
	if d.cfg.Orderer == nil {
		d.cfg.Orderer = NewFair(func() QueueLimits {
			ctx, cancel := context.WithTimeout(context.Background(), queueLimitsTimeout)
			defer cancel()
			return d.queueLimits(ctx)
		}, d.cfg.Clock).WithCost(func() func(string) int64 {
			ctx, cancel := context.WithTimeout(context.Background(), queueLimitsTimeout)
			defer cancel()
			return RuntimeCost(ctx, d.cfg.Store)
		})
	}
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

var errClosed = errors.New("dispatch: the dispatcher is closed")

// Close refuses new work, lets a running tick finish its admissions, resolves
// the waiting callers with an error and waits for the deliveries.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	d.closed = true
	d.mu.Unlock()
	d.stop()
	// An in-flight tick commits and registers its deliveries before this
	// returns from Lock; later ticks see closed and do nothing. Waiting for the
	// deliveries happens outside the lock, or every Submit loop would block in
	// Tick and never see the dispatcher close.
	d.tickM.Lock()
	d.quiesced = true
	d.tickM.Unlock()
	d.wg.Wait()
}

// Submit queues the job and blocks until it holds a lease, ctx is done
// (ctx.Err()), or the dispatcher refuses it (E618, E693, or a store error).
func (d *Dispatcher) Submit(ctx context.Context, j Job) (Admitted, error) {
	e, err := d.enqueueCtx(ctx, j)
	if err != nil {
		return Admitted{}, err
	}
	for {
		wake := d.wakeCh() // taken before due(): a Wake between the two is not lost
		if d.due() {
			_ = d.Tick(ctx) // a failed tick leaves the job queued; foreground world errors finish it
		}
		select {
		case <-e.done:
			return e.res, e.err
		case <-ctx.Done():
			return d.drop(e, ctx.Err())
		case <-d.life.Done():
			return d.drop(e, errClosed)
		case <-wake:
		case <-time.After(d.cfg.Poll):
		}
	}
}
