package coordinator

// Purpose: the Reconciler registry and schedule of the V15 coordinator (P7-NODE-10, scenario audit P5,
// HOST D19). A Reconciler finds triggers the webhooks missed (P7-HOST-17 implements the GitHub sweep);
// this package only runs it and feeds its triggers through the same dedup.
// Inputs:  Reconciler implementations (registered or in Config), the injected clock.
// Outputs: one pass at start and one per interval for each Reconciler.
// Constraints: the interval is Every() when the Reconciler declares one (minimum 15 s) and 5 minutes
//          otherwise; each Reconciler has its own loop, so two passes of one Reconciler never overlap and a
//          slow pass delays only its own next one; a pass error is logged by name, never fatal.

import (
	"context"
	"sync"
	"time"
)

// Intervals of the schedule.
const (
	DefaultReconcileEvery = 5 * time.Minute
	MinReconcileEvery     = 15 * time.Second
)

// Enqueuer is what a Reconciler uses to feed a trigger into the intake.
type Enqueuer interface {
	Enqueue(ctx context.Context, d Delivery) (Outcome, error)
}

// Reconciler is a source of missed triggers.
type Reconciler interface {
	Name() string
	Reconcile(ctx context.Context, enq Enqueuer) error
}

// Everyer is implemented by a Reconciler that wants its own interval.
type Everyer interface{ Every() time.Duration }

var reconcilers struct {
	sync.Mutex
	list []Reconciler
}

// RegisterReconciler adds a Reconciler to every coordinator started afterwards.
func RegisterReconciler(r Reconciler) error {
	if r == nil || r.Name() == "" {
		return newErr("", ReasonInvalidConfig, "reconciler needs a name")
	}
	reconcilers.Lock()
	defer reconcilers.Unlock()
	for _, have := range reconcilers.list {
		if have.Name() == r.Name() {
			return newErr("", ReasonDuplicateName, "reconciler %q is already registered", r.Name())
		}
	}
	reconcilers.list = append(reconcilers.list, r)
	return nil
}

// interval is the cadence of r.
func interval(r Reconciler) time.Duration {
	e, ok := r.(Everyer)
	if !ok || e.Every() <= 0 {
		return DefaultReconcileEvery
	}
	return max(e.Every(), MinReconcileEvery)
}

// runReconcilers runs every Reconciler on its own cadence until ctx ends.
func (c *Coordinator) runReconcilers(ctx context.Context) {
	reconcilers.Lock()
	all := append(append([]Reconciler(nil), reconcilers.list...), c.cfg.Reconcilers...)
	reconcilers.Unlock()
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, r := range all {
		wg.Add(1)
		go func(r Reconciler) {
			defer wg.Done()
			every := interval(r)
			for {
				if err := r.Reconcile(ctx, c); err != nil && ctx.Err() == nil {
					c.cfg.Logf("reconciler %s failed: %v", r.Name(), err)
				}
				select {
				case <-c.cfg.Clock.After(every):
				case <-ctx.Done():
					return
				}
			}
		}(r)
	}
}
