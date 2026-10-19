package drain

// Purpose: the graceful drain state machine (P7-NODE-20, Epic D7): on SIGTERM the intake stops, running
//          leases finish until the deadline, then each remaining lease is cancelled and handed back only
//          after the agent acknowledged the cancel or the lease expired.
// Inputs:  Config (intake, leases, canceller, clock), a term channel for Serve, Ack calls from the agent API.
// Outputs: Report with one Result per lease that was still running when the drain began; ExitCode.
// Constraints: a release job and an idempotent:false job are never cancelled or requeued, they go to
//          needs-operator; no HandBack call happens before an ack or the lease deadline, so the old and
//          the new attempt never overlap; every goroutine started here is awaited before Run returns;
//          the clock is injected so tests are deterministic.

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Drainer runs one drain.
type Drainer struct {
	cfg  Config
	once sync.Once
	mu   sync.Mutex
	acks map[string]chan struct{}
}

// New fills the defaults of cfg. Intake and Leases are required.
func New(cfg Config) (*Drainer, error) {
	if cfg.Intake == nil || cfg.Leases == nil {
		return nil, errors.New("drain: intake and leases are required")
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.Deadline <= 0 {
		cfg.Deadline = DefaultDeadline
	}
	if cfg.Poll <= 0 {
		cfg.Poll = time.Second
	}
	if cfg.AckTimeout <= 0 {
		cfg.AckTimeout = 2 * time.Minute
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	return &Drainer{cfg: cfg, acks: map[string]chan struct{}{}}, nil
}

// Ack records the agent's acknowledgement of a cancel. It counts only for a lease the drain already
// cancelled; an early or unknown ack is dropped. It never blocks.
func (d *Drainer) Ack(leaseID string) {
	d.mu.Lock()
	ch := d.acks[leaseID]
	d.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (d *Drainer) ackChan(id string) chan struct{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	ch := make(chan struct{}, 1)
	d.acks[id] = ch
	return ch
}

// Serve waits for term (SIGTERM) or ctx, then drains. A ctx that ends first returns without draining.
func (d *Drainer) Serve(ctx context.Context, term <-chan struct{}) (Report, error) {
	select {
	case <-term:
		return d.Run(ctx)
	case <-ctx.Done():
		return Report{}, nil
	}
}

// Run drains: stop intake, let leases finish until the deadline, settle what remains.
func (d *Drainer) Run(ctx context.Context) (Report, error) {
	d.once.Do(d.cfg.Intake.Stop)
	rep := Report{Started: d.cfg.Clock.Now()}
	deadline := rep.Started.Add(d.cfg.Deadline)
	first, err := d.cfg.Leases.Running(ctx)
	if err != nil {
		return rep, err
	}
	var left []Lease
	for {
		if left, err = d.cfg.Leases.Running(ctx); err != nil {
			return rep, err
		}
		if len(left) == 0 {
			break
		}
		now := d.cfg.Clock.Now()
		if !now.Before(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return d.finish(rep, first, left, nil), ctx.Err()
		case <-d.cfg.Clock.After(min(d.cfg.Poll, deadline.Sub(now))):
		}
	}
	sort.Slice(left, func(i, j int) bool { return left[i].ID < left[j].ID })
	results := make([]Result, len(left))
	var wg sync.WaitGroup
	for i, l := range left {
		wg.Add(1)
		go func(i int, l Lease) {
			defer wg.Done()
			results[i] = d.settle(ctx, l)
		}(i, l)
	}
	wg.Wait()
	return d.finish(rep, first, left, results), nil
}

// finish builds the report: leases that left the running set on their own are finished, the rest carry
// the settled result.
func (d *Drainer) finish(rep Report, first, left []Lease, settled []Result) Report {
	byID := map[string]Result{}
	for _, r := range settled {
		byID[r.LeaseID] = r
	}
	still := map[string]bool{}
	for _, l := range left {
		still[l.ID] = true
	}
	all := append([]Lease(nil), first...)
	known := map[string]bool{}
	for _, l := range first {
		known[l.ID] = true
	}
	for _, l := range left { // a lease that appeared after the first read still gets a result
		if !known[l.ID] {
			all = append(all, l)
		}
	}
	for _, l := range all {
		switch {
		case !still[l.ID]:
			rep.Results = append(rep.Results, Result{LeaseID: l.ID, Outcome: OutcomeFinished})
		case byID[l.ID].LeaseID != "":
			rep.Results = append(rep.Results, byID[l.ID])
		default:
			rep.Results = append(rep.Results, Result{LeaseID: l.ID, Outcome: OutcomeAbandoned})
		}
	}
	sort.Slice(rep.Results, func(i, j int) bool { return rep.Results[i].LeaseID < rep.Results[j].LeaseID })
	rep.Ended = d.cfg.Clock.Now()
	return rep
}

// ExitCode is 0 when every lease was settled or deliberately left, 1 when a lease hit an error.
func (r Report) ExitCode() int {
	if r.Count(OutcomeError) > 0 {
		return 1
	}
	return 0
}
