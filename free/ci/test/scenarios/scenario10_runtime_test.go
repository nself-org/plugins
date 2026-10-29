//go:build scenarios

package scenarios

// Purpose: Epic acceptance 4 with UNEQUAL job runtimes (D-1411). Project alpha submits few long jobs and
// project beta many short ones; dispatch.Fair weighs each job by its project's settled mean runtime
// (telemetry estimators), so the served RUNTIME of the admission prefixes stays within the Epic's bound,
// `max job runtime x slots`, while serving by job count alone would break it.
// Inputs:  the scenario 10 rig (6 slots), estimators of 900 s for alpha and 30 s for beta.
// Outputs: assertions only.
// Constraints: no sleeps; the test frees capacity and ticks the dispatcher itself, as scenario 10 does.

import (
	"context"
	"fmt"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// s10SeedRuntime records a settled runtime estimator for project.
func s10SeedRuntime(t testing.TB, st *store.Store, project string, ms int64) {
	t.Helper()
	err := st.TelRecordOutcome(context.Background(), store.TelOutcome{AttemptID: "seed-" + project, Project: project, JobKey: "job", RunnerClass: "local", Result: "passed", RuntimeMs: ms, TS: 1},
		func(store.TelEstimator, bool) store.TelEstimator {
			return store.TelEstimator{Project: project, JobKey: "job", RunnerClass: "local", RuntimeMs: ms, N: 3, Updated: 1}
		})
	if err != nil {
		t.Fatal(err)
	}
}

// s10CheckServedRuntime returns a violation for every admission prefix where both projects still had
// queued jobs and their served runtime differs by more than the longest job runtime x slots.
func s10CheckServedRuntime(log []s10Event, total map[string]int, runtime map[string]int64, slots int) []string {
	served, count := map[string]int64{}, map[string]int{}
	var longest int64
	for _, ms := range runtime {
		longest = max(longest, ms)
	}
	var out []string
	for i, e := range log {
		served[e.Project] += runtime[e.Project]
		count[e.Project]++
		if count["alpha"] < total["alpha"] && count["beta"] < total["beta"] {
			if gap := max(served["alpha"]-served["beta"], served["beta"]-served["alpha"]); gap > longest*int64(slots) {
				out = append(out, fmt.Sprintf("after admission %d: alpha served %d ms, beta %d ms (gap %d ms, bound %d ms)", i+1, served["alpha"], served["beta"], gap, longest*int64(slots)))
			}
		}
	}
	return out
}

func TestScenario10UnequalRuntimesStayWithinServedRuntimeBound(t *testing.T) {
	runtime := map[string]int64{"alpha": 900_000, "beta": 30_000}
	total := map[string]int{"alpha": 8, "beta": 40}
	// the check discriminates: serving the same queue by job count (alternating) breaks the bound
	var alternating []s10Event
	for i := 0; i < 16; i++ {
		alternating = append(alternating, s10Event{Project: []string{"alpha", "beta"}[i%2]})
	}
	if len(s10CheckServedRuntime(alternating, total, runtime, 6)) == 0 {
		t.Fatal("the served-runtime check accepts job-count fairness: it cannot tell the orderers apart")
	}
	rig := newS10Rig(t, s10Runners(), nil, nil)
	for p, ms := range runtime {
		s10SeedRuntime(t, rig.st, p, ms)
	}
	for i := 0; i < total["alpha"]; i++ { // the long jobs arrive first, so arrival order is no help either
		rig.submit("alpha", "")
	}
	for i := 0; i < total["beta"]; i++ {
		rig.submit("beta", "")
	}
	n := total["alpha"] + total["beta"]
	rig.waitQueued(n)
	rig.release()
	rig.waitAdmitted(6)
	rig.tick()
	for next := 0; ; next++ {
		log := rig.sink.events(dispatch.OutcomeAdmitted)
		if v := append(s10CheckCapacity(rig.ctx, rig.st, rig.trueSlots, 8), s10CheckServedRuntime(log, total, runtime, 6)...); len(v) > 0 {
			t.Fatalf("after %d finished: %v\nadmission order: %+v", next, v, log)
		}
		if want := min(n, 6+next); len(log) != want {
			t.Fatalf("after %d finished: %d admitted, want %d", next, len(log), want)
		}
		if next == n {
			break
		}
		rig.finish(log[next].Key)
	}
	if v := rig.sink.violations(); len(v) > 0 {
		t.Fatalf("breach seen at an admission: %v", v)
	}
	rig.cancel()
	rig.wg.Wait()
	close(rig.errs)
	for err := range rig.errs {
		t.Fatalf("a submit failed: %v", err)
	}
}
