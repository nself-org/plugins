package schedule

// Purpose: the NODE-10 Reconciler that fires scheduled pipelines (P7-HOST-20, Epic D10). On each tick
// it finds, for every entry of triggers.schedules, the latest scheduled time in (mark, now] and enqueues
// it once through the coordinator's intake as source "schedule", trigger kind schedule, auth per-repo
// (operator scope). Times missed while the coordinator was down collapse to that one fire.
// Inputs:  entries (CurrentEntries by default), the host-provider marks, a Heads resolver for the
//          commit to build, an injected clock, the coordinator's Enqueuer.
// Outputs: one Delivery per due entry with delivery id schedule:<repo>:<pipeline>:<RFC3339 time>; the
//          mark advances to the fired time only after the intake accepted it (an enqueue, a duplicate
//          or a coalesce all count).
// Constraints: no timer and no goroutine of its own: the coordinator's tick is the only clock source
//          (resolution 5 minutes, documented). An entry seen for the first time records the current
//          time as its mark and fires nothing, so enabling a schedule never replays the past. A crash
//          between the enqueue and the mark write re-sends the same delivery id on the next tick and
//          the intake drops it as a duplicate. One failing entry never stops the others; the failures
//          are returned together. Dormant unless compat.V15().

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	// Name is the Reconciler name and the provider_mark name the marks are kept under.
	Name = "schedule"
	// Source is the Delivery source.
	Source = "schedule"
	// Class is the trigger class: distinct from "change", so a scheduled run is never coalesced into
	// the push run of the same commit.
	Class = "schedule"
	// Every is the tick the schedule is evaluated on.
	Every = coordinator.DefaultReconcileEvery
)

// Marks keeps the last accounted-for time per entry. *store.Store satisfies it (host-provider set).
type Marks interface {
	ProviderMarkGet(ctx context.Context, name, key string) (time.Time, bool, error)
	ProviderMarkPut(ctx context.Context, name, key string, at time.Time) error
}

// Heads resolves the commit a scheduled run builds (the head of the repo's default branch).
type Heads interface {
	Head(ctx context.Context, repo, pipeline string) (revision string, err error)
}

// Reconciler is the schedule source. Zero Marks, Heads or Entries make a pass an error, never a
// silent skip, except that no entries is a pass that does nothing.
type Reconciler struct {
	Entries func(context.Context) ([]Entry, error) // default CurrentEntries
	Marks   Marks
	Heads   Heads
	Clock   func() time.Time // default time.Now
}

var _ coordinator.Reconciler = (*Reconciler)(nil)
var _ coordinator.Everyer = (*Reconciler)(nil)
var _ Marks = (*store.Store)(nil)

// Name implements coordinator.Reconciler.
func (*Reconciler) Name() string { return Name }

// Every implements coordinator.Everyer.
func (*Reconciler) Every() time.Duration { return Every }

// markKey names an entry; the cron text is normalized so spacing never makes a second entry.
func markKey(e Entry) string {
	return e.Repo + "|" + e.Pipeline + "|" + strings.Join(strings.Fields(e.Cron), " ")
}

// DeliveryID is the dedup key of one scheduled time of one entry.
func DeliveryID(repo, pipeline string, at time.Time) string {
	return "schedule:" + repo + ":" + pipeline + ":" + at.UTC().Format(time.RFC3339)
}

// Due returns the scheduled time to fire for spec: the latest one in (mark, now], or the zero time.
// Missed times collapse into it.
func Due(spec Spec, mark, now time.Time) time.Time {
	if p := spec.Prev(now); p.After(mark) {
		return p
	}
	return time.Time{}
}

// Reconcile implements coordinator.Reconciler.
func (r *Reconciler) Reconcile(ctx context.Context, enq coordinator.Enqueuer) error {
	if !compat.V15() {
		return nil
	}
	entries := r.Entries
	if entries == nil {
		entries = CurrentEntries
	}
	list, err := entries(ctx)
	if err != nil || len(list) == 0 {
		return err
	}
	if r.Marks == nil || r.Heads == nil {
		return errors.New("schedule: no marks store or head resolver installed")
	}
	clock := r.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock().UTC()
	var errs []error
	for _, e := range list {
		if err := r.fire(ctx, enq, e, now); err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", e.Repo, e.Pipeline, err))
		}
	}
	return errors.Join(errs...)
}

func (r *Reconciler) fire(ctx context.Context, enq coordinator.Enqueuer, e Entry, now time.Time) error {
	spec, err := e.Spec()
	if err != nil {
		return err
	}
	key := markKey(e)
	mark, ok, err := r.Marks.ProviderMarkGet(ctx, Name, key)
	if err != nil {
		return err
	}
	if !ok {
		return r.Marks.ProviderMarkPut(ctx, Name, key, now)
	}
	at := Due(spec, mark, now)
	if at.IsZero() {
		return nil
	}
	rev, err := r.Heads.Head(ctx, e.Repo, e.Pipeline)
	if err != nil {
		return fmt.Errorf("resolve head: %w", err)
	}
	d, err := delivery(e, rev, at, now)
	if err != nil {
		return err
	}
	if _, err := enq.Enqueue(ctx, d); err != nil {
		return err
	}
	return r.Marks.ProviderMarkPut(ctx, Name, key, at)
}

func delivery(e Entry, revision string, at, now time.Time) (coordinator.Delivery, error) {
	id := DeliveryID(e.Repo, e.Pipeline, at)
	facts := model.TriggerFacts{Schema: "ci.trigger-facts/v1", Source: Source, Kind: "schedule", DeliveryID: id, ReceivedAt: now,
		Auth: "per-repo", Repo: e.Repo, Revision: revision, RevisionSource: model.FactCaller}
	raw, err := json.Marshal(facts)
	if err != nil {
		return coordinator.Delivery{}, err
	}
	return coordinator.Delivery{Source: Source, DeliveryID: id, Project: e.Repo, Revision: revision, TriggerClass: Class, Facts: raw, Pipeline: e.Pipeline}, nil
}

var runtime struct {
	sync.Mutex
	r *Reconciler
}

// Default returns the process-wide Reconciler the init() registered; wire its Marks and Heads with
// Use before the coordinator starts.
func Default() *Reconciler {
	runtime.Lock()
	defer runtime.Unlock()
	if runtime.r == nil {
		runtime.r = &Reconciler{}
	}
	return runtime.r
}

// Use sets the marks store and head resolver of the default Reconciler (the coordinator start does
// this once, with its *store.Store).
func Use(m Marks, h Heads) {
	r := Default()
	runtime.Lock()
	defer runtime.Unlock()
	r.Marks, r.Heads = m, h
}

func init() {
	if err := coordinator.RegisterReconciler(Default()); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
}
