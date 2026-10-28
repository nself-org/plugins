package burst

// Purpose: the sweeper (Epic D12): list what each provider holds under this
// coordinator's labels, judge every machine with SweepDecide, and delete the
// ones that must go. It runs on start and every five minutes.
// Inputs:  dryRun. Outputs: a SweepReport (every action, deleted or only planned).
// Constraints: only machines labelled role=ci-ephemeral with this coordinator's
// id are considered, and never one in the exclusion set (an unreadable set
// stops the whole sweep); --dry-run lists what would be deleted, including
// intent-less machines, and changes nothing, not even an alert; an intent-less
// machine is alerted once and deleted after max_lifetime + grace; a failure on
// one provider does not stop the others; a real sweep also prunes the decision
// log (90 days, 20 000 rows) and the alert marks.

import (
	"context"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// SweepAction is one machine and what the sweep does (or would do) with it.
type SweepAction struct {
	Provider, Resource, Intent, Reason string
	Delete, Intentless                 bool
}

// SweepReport lists the actions of one sweep.
type SweepReport struct {
	Actions []SweepAction
	Deleted int
}

// Sweep runs one pass over every provider.
func (e *Engine) Sweep(ctx context.Context, dryRun bool) (SweepReport, error) {
	var rep SweepReport
	ex, err := e.o.Exclusion(ctx)
	if err != nil {
		return rep, fmt.Errorf("burst: the exclusion set is unreadable, so nothing is swept: %w", err)
	}
	var errs []error
	for _, id := range e.providerIDs() {
		p := e.providers[id]
		listed, err := p.List(ctx, OwnLabels(e.o.Coordinator))
		if err != nil {
			errs = append(errs, fmt.Errorf("sweep %s: %s", id, errClass(err)))
			continue
		}
		for _, r := range listed {
			act, err := e.judge(ctx, p, ex, r)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if act.Reason == SweepExcluded && Mine(r.Labels, e.o.Coordinator) && !dryRun {
				e.alertOnce(ctx, Alert{Kind: AlertExcluded, Provider: id, Intent: r.Labels[LabelIntent], Resource: r.ID, Detail: "carries this coordinator's labels but is in the deploy-host exclusion set; left alone"})
			}
			if act.Reason == SweepForeign || act.Reason == SweepExcluded {
				continue // not ours to report as an action
			}
			if !dryRun {
				if act.Intentless && !act.Delete {
					e.alertOnce(ctx, Alert{Kind: AlertOrphan, Provider: id, Resource: r.ID, Detail: "labelled for this coordinator but no intent row; deleted after max_lifetime + grace"})
				}
				if act.Delete {
					if derr := e.remove(ctx, p, r, act); derr != nil {
						errs = append(errs, derr)
					} else {
						rep.Deleted++
					}
				}
			}
			rep.Actions = append(rep.Actions, act)
		}
	}
	if !dryRun {
		before := e.now().Add(-DecisionRetention).UnixMilli()
		if _, err := e.o.Store.BurstPrune(ctx, before, DecisionMaxRows); err != nil {
			errs = append(errs, errors.New("sweep: the decision log could not be pruned"))
		}
	}
	return rep, errors.Join(errs...)
}

func (e *Engine) judge(ctx context.Context, p Provider, ex deployguard.Set, r Resource) (SweepAction, error) {
	token := r.Labels[LabelIntent]
	if token != "" && !labelSafe(token) {
		token = "" // a provider-controlled label never reaches a query or an audit row unless it is a plain token
	}
	act := SweepAction{Provider: p.ID(), Resource: r.ID, Intent: token}
	f := SweepFacts{Mine: Mine(r.Labels, e.o.Coordinator), Excluded: excluded(ex, r), MaxLifetimeMs: e.cfg.MaxLifetime.Milliseconds(), GraceMs: e.cfg.Grace.Milliseconds()}
	if !r.CreatedAt.IsZero() {
		f.AgeMs = e.now().Sub(r.CreatedAt).Milliseconds()
	}
	if token != "" {
		in, ok, err := e.o.Store.BurstIntent(ctx, token)
		if err != nil {
			return act, fmt.Errorf("sweep %s: intent unreadable", p.ID())
		}
		if ok && in.Coordinator != e.o.Coordinator {
			f.Mine = false // the label says ours, the intent row says another coordinator's: touch nothing
		}
		f.HasIntent, f.IntentState = ok, in.State
		if ok && r.CreatedAt.IsZero() {
			// A provider that reports no creation time (the conformance kit forbids it) must not read as "infinitely old":
			// the intent's own birth is the best clock there is.
			f.AgeMs = max(e.now().UnixMilli()-in.CreatedAt, 0)
		}
	}
	v := SweepDecide(f)
	act.Delete, act.Reason, act.Intentless = v.Delete, v.Reason, !f.HasIntent
	return act, nil
}

// remove executes one deletion through the intent's teardown when it has a live
// or tearing-down intent, else directly (terminal intent, or no intent at all).
func (e *Engine) remove(ctx context.Context, p Provider, r Resource, a SweepAction) error {
	if a.Reason == SweepAgeExpired || a.Reason == SweepIntentTearing {
		if err := e.Teardown(ctx, a.Intent, "sweep: "+a.Reason); err != nil {
			return err
		}
		return nil
	}
	if err := p.Teardown(ctx, r.ID); err != nil && !errors.Is(err, ErrNotFound) {
		return teardownFailed(err, "sweep %s: resource %s: %s", p.ID(), r.ID, errClass(err))
	}
	return e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error {
		return t.AppendAudit("burst", "burst.sweep_deleted", r.ID, fmt.Sprintf("provider=%s intent=%s reason=%s", p.ID(), a.Intent, a.Reason))
	})
}

// alertOnce tells the operator once about a machine (kind and resource are the dedupe key). The mark is
// kept in the store, so a restart does not repeat it; when the store cannot answer, the alert is sent
// (a repeat is better than silence) and the in-memory mark still limits it to once per run.
func (e *Engine) alertOnce(ctx context.Context, a Alert) {
	key := a.Kind + "/" + a.Resource
	e.mu.Lock()
	seen := e.orphans[key]
	e.orphans[key] = true
	e.mu.Unlock()
	if seen {
		return
	}
	if first, err := e.o.Store.BurstAlertOnce(ctx, a.Kind, a.Resource); err == nil && !first {
		return
	}
	if e.o.Alert != nil {
		e.o.Alert(a)
	}
}
