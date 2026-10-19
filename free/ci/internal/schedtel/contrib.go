package schedtel

// Purpose: the `schedtel` world contributor. It owns sched.World.Estimates and
// fills it, for every runner and every attempt now queued, from the stored
// outcomes: runtime and cold start from the EWMA estimators, reliability from
// the runner's last 20 attempts, wait from queue position x mean runtime / slots.
// Inputs:  the world with its runners built; the store of the current run.
// Outputs: Estimates[runner id][attempt id] (sched keys a sample by job key,
// which the run hook sets to the attempt id).
// Constraints: a figure resting on fewer than MinSamples samples is left out,
// so the scheduler uses its own stand-in and marks it unknown; no store, no
// estimates; a store failure leaves Estimates empty (the scheduler then uses
// its unknown stand-ins) and is reported as a note on the run by the finalizer,
// because telemetry must not stop a placement. The samples cover every queued
// attempt in the store, so the world digest changes when an unrelated pipeline
// queues a job (expected, see the handoff).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

func contribute(ctx context.Context, w *sched.World) error {
	st := storeIn(ctx)
	if st == nil {
		return nil
	}
	if err := estimate(ctx, w, st); err != nil {
		w.Estimates = nil
		noteFailure(err)
	}
	return nil
}

func estimate(ctx context.Context, w *sched.World, st *store.Store) error {
	ests, err := st.TelEstimators(ctx)
	if err != nil {
		return err
	}
	queued, err := st.TelQueued(ctx)
	if err != nil {
		return err
	}
	byKey := map[[3]string]store.TelEstimator{}
	sum, count := map[string]int64{}, map[string]int64{} // per runner class: runtime sum and count of settled estimators
	for _, e := range ests {
		byKey[[3]string{e.Project, e.JobKey, e.RunnerClass}] = e
		if e.N >= MinSamples {
			sum[e.RunnerClass] += e.RuntimeMs
			count[e.RunnerClass]++
		}
	}
	out := map[string]map[string]sched.Sample{}
	for _, r := range append(append([]sched.Runner(nil), w.Runners...), w.Hosted...) {
		class := classOf(r)
		infra, err := reliability(ctx, st, r.ID)
		if err != nil {
			return err
		}
		wait := waitMs(r, sum[class], count[class])
		for _, q := range queued {
			var s sched.Sample
			if e, ok := byKey[[3]string{q.Project, q.JobName, class}]; ok {
				if e.N >= MinSamples {
					v := e.RuntimeMs
					s.RuntimeMs = &v
				}
				if e.ColdN >= MinSamples {
					v := e.ColdStartMs
					s.ColdStartMs = &v
				}
			}
			s.WaitMs, s.InfraFailurePermille = wait, infra
			if s.RuntimeMs == nil && s.ColdStartMs == nil && s.WaitMs == nil && s.InfraFailurePermille == nil {
				continue
			}
			s.Confidence = "estimated"
			if out[r.ID] == nil {
				out[r.ID] = map[string]sched.Sample{}
			}
			out[r.ID][q.ID] = s
		}
	}
	w.Estimates = out
	return nil
}

// reliability is the share (permille) of the runner's last 20 attempts that
// failed for infrastructure reasons; nil under MinSamples attempts.
func reliability(ctx context.Context, st *store.Store, runner string) (*int64, error) {
	classes, err := st.TelRunnerResults(ctx, runner, ReliabilityWindow)
	if err != nil || len(classes) < MinSamples {
		return nil, err
	}
	bad := int64(0)
	for _, c := range classes {
		if c == "infra" || c == "lost" {
			bad++
		}
	}
	v := bad * 1000 / int64(len(classes))
	return &v, nil
}

// waitMs is the runner's queue position x the mean runtime of its class / its
// slots; nil when no runtime of the class is settled.
func waitMs(r sched.Runner, sum, count int64) *int64 {
	if count == 0 {
		return nil
	}
	n, _ := sched.Mul(r.QueuedAhead, sum/count)
	v := n / max(int64(r.Avail.Slots), 1)
	return &v
}

func registerContributor() error {
	return world.Register(world.Contributor{Name: "schedtel", Field: "Estimates", Fn: contribute})
}
