package burst

// Purpose: Provision, the wait-versus-provision decision turned into a machine
// (Epic D12): decide, log the decision with every input, commit the intent and
// its lifetime reservation in one transaction, and only then call the provider.
// Inputs:  a Request for one queued job.
// Outputs: the Decision, the intent token and the Resource, or an E696.
// Constraints: no provider call before the intent commits; a retry for the same
// token lists by the intent label and adopts instead of creating a second
// machine; a provider failure after the intent is cleaned up through Teardown
// (and left to the sweeper when that cannot finish); a lost response leaves a
// labelled machine and an intent row, never an unaccounted machine.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Request asks for one machine for one queued job.
type Request struct {
	Provider, Pipeline, JobKey string
	Spec                       Spec
	// ExpectedWaitMs is the job's expected wait; QueuedDemand the queued jobs the machine would serve.
	ExpectedWaitMs int64
	QueuedDemand   int
}

// Result is the outcome of Provision. Token and Resource are empty when the decision was no.
type Result struct {
	Decision Decision
	Token    string
	Resource Resource
}

// Provision decides and, when allowed, creates one machine.
func (e *Engine) Provision(ctx context.Context, req Request) (Result, error) {
	p, ok := e.providers[req.Provider]
	if !ok || req.JobKey == "" {
		return Result{}, refused("provider.unknown", nil, "provider %q is not configured or the job has no key", req.Provider)
	}
	// One provision decides and commits at a time, so two callers cannot both see an empty window or a free slot;
	// commitIntent re-counts inside its transaction as well, which also covers a second process on the same store.
	e.provMu.Lock()
	defer e.provMu.Unlock()
	pol, err := e.o.Spend(ctx)
	if err != nil {
		return Result{}, refused("policy.unreadable", err, "the spend policy could not be read")
	}
	est, err := p.Estimate(ctx, req.Spec)
	if err != nil {
		est = Estimate{} // unknown latency takes the default; the decision does not wait on an estimate
	}
	now := e.now()
	token, err := e.o.NewToken(now)
	if err != nil {
		return Result{}, refused("token", err, "no intent token")
	}
	counts, err := e.o.Store.BurstCountsOf(ctx, p.ID(), req.JobKey, now.UnixMilli()-WindowMs(est.LatencyMs))
	if err != nil {
		return Result{}, refused("store", err, "counters unreadable")
	}
	br, err := e.o.Store.Breaker(ctx, p.ID())
	if err != nil {
		return Result{}, refused("store", err, "breaker unreadable")
	}
	cloud := p.Class() == ClassCloud
	in := Input{Provider: p.ID(), Cloud: cloud, Team: e.o.Team(), Enabled: pol.Burst.Enabled[p.ID()], Paid: cloud,
		PaidAllowed: schedpolicy.PaidAllowed(pol, p.ID()), ExpectedWaitMs: req.ExpectedWaitMs, LatencyMs: est.LatencyMs, MarginMs: pol.Burst.MarginMs,
		QueuedDemand: req.QueuedDemand, InFlight: counts.InFlight, JobWindowCount: counts.JobWindow, Open: counts.Open, MaxConcurrent: int(pol.Burst.MaxConcurrent),
		BreakerOpen: br.OpenUntil > now.UnixMilli(), TeardownFailed: counts.TeardownFailed, ReservationFits: true}
	if cloud {
		if ferr := e.fits(ctx, token, p.ID(), req.Pipeline); ferr != nil {
			in.ReservationFits, in.FitReason = false, fitReason(ferr)
		}
	}
	d := Decide(in)
	raw, _ := json.Marshal(d.Input)
	if err = e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error {
		return t.PutDecision(now.UnixMilli(), p.ID(), req.JobKey, d.Allow, d.Reason, string(raw))
	}); err != nil {
		return Result{Decision: d}, refused("store", err, "the decision could not be recorded")
	}
	if !d.Allow {
		return Result{Decision: d}, refused(d.Reason, nil, "provider %s, job %s", p.ID(), req.JobKey)
	}
	if err = e.commitIntent(ctx, token, p, req, est, now.UnixMilli(), int(pol.Burst.MaxConcurrent)); err != nil {
		return Result{Decision: d}, err
	}
	if err = e.at(PointAfterIntent); err != nil {
		return Result{Decision: d, Token: token}, err
	}
	res, err := e.create(ctx, p, token, req.Spec)
	if err != nil {
		if errors.Is(err, ErrKilled) {
			return Result{Decision: d, Token: token}, err
		}
		_ = e.Teardown(context.WithoutCancel(ctx), token, "provision failed")
		return Result{Decision: d, Token: token}, refused("provider.error", err, "%s: %s", p.ID(), errClass(err))
	}
	if err = e.at(PointAfterCreate); err != nil {
		return Result{Decision: d, Token: token}, err
	}
	if err = e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error { return t.SetResource(token, res.ID, t.NowMs()) }); err != nil {
		return Result{Decision: d, Token: token, Resource: res}, refused("store", err, "the resource id could not be recorded")
	}
	return Result{Decision: d, Token: token, Resource: res}, nil
}

// fitReason is the placement reason of a refused reservation.
func fitReason(err error) string {
	var se *economics.SpendError
	if errors.As(err, &se) {
		return string(se.Reason)
	}
	return "reservation"
}

var (
	errDryRun    = errors.New("burst: dry run rolled back")
	errCapsMoved = errors.New("burst: the caps changed before the commit")
)

// fits asks economics whether the lifetime reservation would fit, in a transaction that is rolled back.
func (e *Engine) fits(ctx context.Context, token, provider, pipeline string) error {
	err := e.o.Store.LedgerWrite(e.econCtx(ctx), func(t store.LedgerTx) error {
		if err := economics.ReserveResource(e.econCtx(ctx), t, economics.Resource{Intent: token, Provider: provider, Pipeline: pipeline, LifetimeMs: e.cfg.LifetimeMs()}); err != nil {
			return err
		}
		return errDryRun
	})
	if errors.Is(err, errDryRun) {
		return nil
	}
	return err
}

// commitIntent writes the intent row and, for a paid provider, the lifetime reservation in one transaction.
func (e *Engine) commitIntent(ctx context.Context, token string, p Provider, req Request, est Estimate, nowMs int64, maxConc int) error {
	ectx := e.econCtx(ctx)
	err := e.o.Store.LedgerWrite(ectx, func(t store.LedgerTx) error {
		var reserved int64
		if p.Class() == ClassCloud {
			if err := economics.ReserveResource(ectx, t, economics.Resource{Intent: token, Provider: p.ID(), Pipeline: req.Pipeline, LifetimeMs: e.cfg.LifetimeMs()}); err != nil {
				return err
			}
			row, _, err := t.Resource(token)
			if err != nil {
				return err
			}
			reserved = row.ReservedMicro
		}
		counts, err := t.Burst().Counts(p.ID(), req.JobKey, nowMs-WindowMs(est.LatencyMs))
		if err != nil {
			return err
		}
		if counts.JobWindow >= 1 || counts.Open >= max(maxConc, 0) || req.QueuedDemand <= counts.InFlight {
			return errCapsMoved
		}
		lease := store.BurstLease{IdleUntil: nowMs + e.cfg.IdleTimeout.Milliseconds() + EffectiveLatency(est.LatencyMs), MaxUntil: nowMs + e.cfg.MaxLifetime.Milliseconds()}
		return t.Burst().PutIntent(store.BurstIntent{Token: token, Coordinator: e.o.Coordinator, Provider: p.ID(), Pipeline: req.Pipeline, JobKey: req.JobKey,
			Platform: req.Spec.Platform, ReservedMicro: reserved, LifetimeMs: e.cfg.LifetimeMs(), CreatedAt: nowMs}, lease)
	})
	var se *economics.SpendError
	if errors.As(err, &se) {
		return refused(string(se.Reason), err, "%s", se.Detail)
	}
	if errors.Is(err, errCapsMoved) {
		return refused(ReasonProviderCap, err, "another provision took the slot first")
	}
	if err != nil {
		return refused("store", err, "the intent could not be committed")
	}
	return nil
}

// create lists by the intent label and adopts a machine that is already there; only
// when there is none does it call Provision (itself idempotent by the label).
func (e *Engine) create(ctx context.Context, p Provider, token string, spec Spec) (Resource, error) {
	found, err := p.List(ctx, map[string]string{LabelIntent: token})
	if err != nil {
		return Resource{}, err // idempotence must not depend on the provider alone: no list, no create
	}
	for _, r := range found {
		if Mine(r.Labels, e.o.Coordinator) && r.Labels[LabelIntent] == token {
			return r, nil
		}
	}
	r, err := p.Provision(ctx, Intent{Token: token, Spec: spec, Labels: IntentLabels(e.o.Coordinator, token)})
	if err != nil {
		return Resource{}, err
	}
	if !Mine(r.Labels, e.o.Coordinator) || r.Labels[LabelIntent] != token {
		return Resource{}, fmt.Errorf("burst: provider %s returned a machine without this intent's labels", p.ID())
	}
	return r, nil
}

// Resume completes the provision of an intent that has no recorded resource (a crash
// after the create): it lists by the intent label and adopts, so no second machine appears.
func (e *Engine) Resume(ctx context.Context, token string, spec Spec) (Resource, error) {
	in, ok, err := e.o.Store.BurstIntent(ctx, token)
	if err != nil || !ok || in.Coordinator != e.o.Coordinator || in.State != store.BurstLive {
		return Resource{}, refused("intent.unavailable", err, "intent %s cannot be resumed", token)
	}
	p, ok := e.providers[in.Provider]
	if !ok {
		return Resource{}, refused("provider.unknown", nil, "provider %q is not configured", in.Provider)
	}
	res, err := e.create(ctx, p, token, spec)
	if err != nil {
		return Resource{}, refused("provider.error", err, "%s: %s", p.ID(), errClass(err))
	}
	if err = e.o.Store.BurstWrite(ctx, func(t store.BurstTx) error { return t.SetResource(token, res.ID, t.NowMs()) }); err != nil {
		return res, refused("store", err, "the resource id could not be recorded")
	}
	return res, nil
}
