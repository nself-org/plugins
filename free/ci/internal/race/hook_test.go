package race

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/run"
)

func TestHookRegistered(t *testing.T) {
	if !run.RaceRegistered() {
		t.Fatal("init did not register the race hook")
	}
}

func TestHookPlansRaces(t *testing.T) {
	no := false
	p := &plan.Plan{Jobs: []plan.PlannedJob{
		{ID: "test", Job: model.Job{}},
		{ID: "ship", Job: model.Job{Kind: "release"}},
		{ID: "once", Job: model.Job{Idempotent: &no}},
	}}
	h := &Hook{}
	h.Race(context.Background(), run.RaceRequest{RunID: "r1", Plan: p})
	if h.Planned("r1") != nil {
		t.Fatal("a run without sched.race.width raced")
	}
	opts := run.Options{PolicyRun: run.RunValues{KeyWidth: json.RawMessage("2")}}
	h.Race(context.Background(), run.RaceRequest{RunID: "r1", Opts: opts, Plan: p})
	if !h.Raced("r1", "test") || h.Raced("r1", "ship") || h.Raced("r1", "once") || h.Raced("r1", "missing") {
		t.Fatalf("plan = %+v", h.Planned("r1"))
	}
	h.Forget("r1")
	if h.Planned("r1") != nil {
		t.Fatal("Forget kept the plan")
	}
	h = &Hook{Enabled: func(run.RaceRequest) bool { return true }}
	h.Race(context.Background(), run.RaceRequest{RunID: "r2", Plan: p})
	if !h.Raced("r2", "test") {
		t.Fatal("Enabled was ignored")
	}
}

func TestRaceJobRefusesRelease(t *testing.T) {
	spec := run.AttemptSpec{Job: &model.Job{Kind: "release"}, N: 1}
	_, err := RaceJob(context.Background(), spec, world(runner("a", "lan", "owned", 1), runner("b", "lan", "owned", 1)), job(), &fakePoster{})
	if err == nil {
		t.Fatal("a release job was raced")
	}
}
