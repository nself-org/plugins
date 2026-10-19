package main

// Purpose: D-1067 regression. `nself ci plan` builds its world without a
// dispatcher, so the schedtel contributor must read the run store from the
// plan's own context and key its samples by the plan's jobs, not only by
// queued attempts. Seeded estimators must reach data.placement.
// Constraints: same git fixture as the placement tests; the store is seeded
// through the store API before the plan runs.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedtel"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func seedEstimator(t *testing.T, home, project, job string, runtimeMs int64) {
	t.Helper()
	st, err := cirun.OpenStore(filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	fold := func(store.TelEstimator, bool) store.TelEstimator {
		return store.TelEstimator{Project: project, JobKey: job, RunnerClass: "local", RuntimeMs: runtimeMs, N: int64(schedtel.MinSamples)}
	}
	o := store.TelOutcome{AttemptID: "seed." + job, Project: project, JobKey: job, Runner: "local", RunnerClass: "local", Result: "passed", RuntimeMs: runtimeMs, TS: 1}
	if err := st.TelRecordOutcome(context.Background(), o, fold); err != nil {
		t.Fatal(err)
	}
}

func TestPlanUsesTelemetryHistory(t *testing.T) {
	repo, home := placementRepo(t)
	project := policy.ProjectIDGit(context.Background(), policy.Runner(report.Capture), repo)
	seedEstimator(t, home, project, "build", 4200)
	var doc model.PlacementPlan
	if err := json.Unmarshal(planJSON(t, repo, true)["placement"], &doc); err != nil {
		t.Fatal(err)
	}
	for _, j := range doc.Jobs {
		if j.ID != "build" {
			if j.Placement != nil && j.Placement.Estimate.RuntimeMs != nil {
				t.Errorf("%s has no history but shows runtime %d", j.ID, *j.Placement.Estimate.RuntimeMs)
			}
			continue
		}
		if j.Placement == nil || j.Placement.Estimate.RuntimeMs == nil || *j.Placement.Estimate.RuntimeMs != 4200 {
			t.Fatalf("build: want runtime 4200 from the seeded estimator, got %+v", j.Placement)
		}
	}
}
