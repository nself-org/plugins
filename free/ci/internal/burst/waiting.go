package burst

// Purpose: the dispatcher side of the burst engine (Epic D12 wiring, D-1329): when a tick leaves a job
// waiting, ask the providers, own machines first and paid ones after, for one machine for it. Decide
// (decide.go) still answers whether waiting or provisioning is right; this file only turns a waiting job
// into a Request and keeps one job from asking on every tick.
// Inputs:  a dispatch.Waiting from the dispatcher's waiting hook.
// Outputs: at most one provisioned machine per job (the intent row); a refused ask leaves a burst_decision row.
// Constraints: the hook returns at once, Provision runs in its own goroutine; one ask per job at a time and
// at most one per AskEvery (a refusal is a decision row, so asking every tick would log a row per tick);
// nothing runs without a bound engine; a job with no attempt id and no key is skipped.

import (
	"context"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
)

// AskEvery is the shortest time between two asks for the same job.
const AskEvery = 30 * time.Second

// askRetention bounds how long a finished ask is remembered.
const askRetention = time.Hour

type askState struct {
	until   time.Time
	running bool
}

// onWaiting is the dispatcher hook: it starts the ask in the background.
func onWaiting(_ context.Context, w dispatch.Waiting) {
	e := current()
	if e == nil {
		return
	}
	key, ok := askKey(w)
	if !ok || !e.claimAsk(key) {
		return
	}
	go e.ask(context.WithoutCancel(context.Background()), key, w)
}

// Waiting asks for a machine for one waiting job and returns when the ask has ended. The dispatcher hook
// calls it in the background; it is exported for the conformance and wiring tests.
func (e *Engine) Waiting(ctx context.Context, w dispatch.Waiting) {
	if key, ok := askKey(w); ok && e.claimAsk(key) {
		e.ask(ctx, key, w)
	}
}

func askKey(w dispatch.Waiting) (string, bool) {
	if w.Job.AttemptID != "" {
		return w.Job.AttemptID, true
	}
	return w.Job.Sched.Key, w.Job.Sched.Key != ""
}

// claimAsk reserves the right to ask for key now.
func (e *Engine) claimAsk(key string) bool {
	now := e.now()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.asks == nil {
		e.asks = map[string]*askState{}
	}
	if st := e.asks[key]; st != nil && (st.running || now.Before(st.until)) {
		return false
	}
	if len(e.asks) > 256 {
		for k, st := range e.asks {
			if !st.running && now.Sub(st.until) > askRetention {
				delete(e.asks, k)
			}
		}
	}
	e.asks[key] = &askState{until: now.Add(AskEvery), running: true}
	return true
}

func (e *Engine) ask(ctx context.Context, key string, w dispatch.Waiting) {
	defer func() {
		e.mu.Lock()
		if st := e.asks[key]; st != nil {
			st.running = false
		}
		e.mu.Unlock()
	}()
	req := w.Job.Sched.Requirements
	spec := Spec{Platform: req.Platform, CPU: int(req.CPU), MemMB: int(req.MemMB), TimeoutMs: w.Job.Sched.TimeoutMs}
	for _, id := range e.providersCheapestFirst() {
		_, err := e.Provision(ctx, Request{Provider: id, Pipeline: w.Pipeline, JobKey: key, Spec: spec, ExpectedWaitMs: w.ExpectedWaitMs, QueuedDemand: w.Demand})
		if err == nil {
			return // a machine is on its way: no second provider for the same job
		}
	}
}

// providersCheapestFirst lists the provider ids with local ones before cloud ones, each group by id.
func (e *Engine) providersCheapestFirst() []string {
	ids := e.providerIDs()
	sort.SliceStable(ids, func(i, j int) bool {
		return (e.providers[ids[i]].Class() == ClassLocal) && (e.providers[ids[j]].Class() != ClassLocal)
	})
	return ids
}
