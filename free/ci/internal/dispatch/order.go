package dispatch

// Purpose: the queue order the dispatcher hands to sched.Place (Epic D7, D9).
// The default Orderer is Fair: strict priority bands, start-time fair queuing
// across projects, then job priority and age, with a starvation ceiling
// (internal/fairq). FIFO (age order) stays for tests and for callers that want
// no fairness.
// Inputs:  the queued jobs. Outputs: the same jobs in placement order.
// Constraints: an Orderer must be deterministic and return exactly the jobs it
// was given; ordering never decides eligibility (sched.Place does), so a
// higher band or priority cannot admit a job no runner may take. Fair keeps the
// virtual clock between ticks and learns which jobs were admitted from the jobs
// that left the queue, so call Order once per tick with the whole live queue
// (Dispatcher.Plan with a hypothetical queue would charge the projects of the
// jobs it leaves out: give Plan a FIFO or a fresh Fair for what-if queues).
// A job weighs its project's mean settled runtime in virtual time (WithCost, fed
// from the telemetry estimators), so a project of long jobs gets fewer starts than
// one of short jobs and served runtime stays balanced (D-1411). Without a cost
// source, or before any estimator has settled, every job weighs 1.

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
)

// Queue limits when no policy supplies them (sched.fairness.starvation_after,
// sched.queue.max_depth).
const (
	DefaultStarvationAfterMs = int64(30 * 60 * 1000)
	DefaultMaxDepth          = int64(500)
)

// QueueLimits is the fairness and depth part of the policy. Zero StarvationAfterMs
// turns promotion off and zero MaxDepth means no cap, as in schedpolicy.
type QueueLimits struct {
	StarvationAfterMs int64
	MaxDepth          int64
}

// DefaultQueueLimits are the registered defaults.
func DefaultQueueLimits() QueueLimits {
	return QueueLimits{StarvationAfterMs: DefaultStarvationAfterMs, MaxDepth: DefaultMaxDepth}
}

// Queued is a job with its place in the queue.
type Queued struct {
	Job        Job
	EnqueuedAt time.Time
	Seq        uint64
}

// Orderer sets the order jobs are placed and admitted in.
type Orderer interface {
	Order([]Queued) []Queued
}

// FIFO orders by age, oldest first.
type FIFO struct{}

// Order implements Orderer; the input is not modified.
func (FIFO) Order(q []Queued) []Queued {
	out := append([]Queued(nil), q...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].EnqueuedAt.Equal(out[j].EnqueuedAt) {
			return out[i].EnqueuedAt.Before(out[j].EnqueuedAt)
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

// Fair is the fair-queuing Orderer.
type Fair struct {
	limits func() QueueLimits
	clock  func() time.Time
	costs  func() func(project string) int64
	mu     sync.Mutex
	state  fairq.State
}

// NewFair returns a Fair orderer. limits is read on every Order (nil means the
// defaults) and clock supplies the time starvation is measured against.
func NewFair(limits func() QueueLimits, clock func() time.Time) *Fair {
	if limits == nil {
		limits = DefaultQueueLimits
	}
	if clock == nil {
		clock = time.Now
	}
	return &Fair{limits: limits, clock: clock}
}

// WithCost sets where job weights come from: costs is called once per Order and
// returns the weight of a job of a project (nil, or a non-positive weight, is 1).
// It returns f.
func (f *Fair) WithCost(costs func() func(project string) int64) *Fair {
	f.costs = costs
	return f
}

// bandOf is the band a queued job is ordered in: the band it declares, else
// interactive for a foreground job, else normal. The run derives the trigger's
// band when it builds the job (attemptBand), so an undeclared band here means
// the caller did not say: never interactive on that alone.
func bandOf(declared string, foreground bool) fairq.Band {
	if b, ok := fairq.Parse(declared); ok {
		return b
	}
	if foreground {
		return fairq.Interactive
	}
	return fairq.Normal
}

// Order implements Orderer; the input is not modified.
func (f *Fair) Order(q []Queued) []Queued {
	var cost func(string) int64
	if f.costs != nil {
		cost = f.costs()
	}
	items := make([]fairq.Item, len(q))
	bySeq := make(map[string]Queued, len(q))
	for i, e := range q {
		key := strconv.FormatUint(e.Seq, 10) // the arrival number is unique; a job key only while queued
		s := e.Job.Sched
		c := int64(1)
		if cost != nil {
			c = max(cost(s.Project), 1)
		}
		items[i] = fairq.Item{
			Key: key, Pipeline: s.Pipeline.ID, Job: s.ID, MatrixKey: s.MatrixKey, Project: s.Project,
			Band: bandOf(s.Pipeline.Band, e.Job.Foreground || s.Pipeline.Foreground), Priority: s.Priority,
			EnqueuedMs: e.EnqueuedAt.UnixMilli(), Seq: e.Seq, Cost: c,
		}
		bySeq[key] = e
	}
	f.mu.Lock()
	ranked, next := fairq.Order(f.state, items, f.clock().UnixMilli(), fairq.Config{StarvationAfterMs: f.limits().StarvationAfterMs})
	f.state = next
	f.mu.Unlock()
	out := make([]Queued, len(ranked))
	for i, r := range ranked {
		out[i] = bySeq[r.Key]
	}
	return out
}
