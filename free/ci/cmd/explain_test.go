package main

// Purpose: tests of `nself ci explain` (P7-SCHED-03): a planned job lists the
// chosen runner and every alternative, the run/job form asks the registered
// DecisionSource and is E692 without one, and an unknown job is E692.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/planview"
)

func explainEnv(t *testing.T, out string) (data planview.Explanation, errCode string) {
	t.Helper()
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("envelope: %v\n%s", err, out)
	}
	if env.Error != nil {
		return data, env.Error.Code
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	return data, ""
}

func TestExplainPlannedJob(t *testing.T) {
	repo, _ := placementRepo(t)
	resetPlanFlags()
	code, out, errs := capture(t, func() int { return runExplain([]string{"build", "--json", "--repo", repo}) })
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, out, errs)
	}
	d, _ := explainEnv(t, out)
	if d.Source != planview.SourcePlan || len(d.Jobs) != 1 || d.Jobs[0].ID != "build" || d.Totals.Placed != 1 {
		t.Fatalf("one planned job expected: %+v", d)
	}
	j := d.Jobs[0]
	if j.Placement == nil || j.Placement.RunnerID != "local" || len(j.Placement.Components) != 9 || len(j.Alternatives) == 0 {
		t.Fatalf("chosen runner, components and alternatives: %+v", j)
	}
	resetPlanFlags()
	code, out, _ = capture(t, func() int { return runExplain([]string{"--repo", repo, "build"}) })
	for _, want := range []string{"job build  placed", "chosen  local", "alt     local  chosen", "wait=", "secret classes project"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("human output lacks %q (exit %d)\n%s", want, code, out)
		}
	}
}

func TestExplainRunWithoutSource(t *testing.T) {
	repo, _ := placementRepo(t)
	planview.Register(nil)
	resetPlanFlags()
	code, out, _ := capture(t, func() int { return runExplain([]string{"run1/build", "--json", "--repo", repo}) })
	_, ec := explainEnv(t, out)
	if code != 1 || ec != "E692" || !strings.Contains(out, "nself ci runs decisions") {
		t.Fatalf("exit %d code %q\n%s", code, ec, out)
	}
	resetPlanFlags()
	code, _, errs := capture(t, func() int { return runExplain([]string{"run1/build", "--repo", repo}) })
	if code != 1 || !strings.Contains(errs, "nself ci runs decisions") {
		t.Fatalf("human: exit %d %s", code, errs)
	}
}

type recordedSource struct{}

func (recordedSource) Decision(_ context.Context, run, job string) (model.PlacementPlan, error) {
	return model.PlacementPlan{Schema: planview.SchemaID, Pipeline: model.PlacementPipelineRef{Name: run}, Jobs: []model.JobPlacement{{ID: job, Status: "waiting"}}}, nil
}

func TestExplainRunWithSource(t *testing.T) {
	repo, _ := placementRepo(t)
	planview.Register(recordedSource{})
	defer planview.Register(nil)
	resetPlanFlags()
	code, out, errs := capture(t, func() int { return runExplain([]string{"run1/build", "--json", "--repo", repo}) })
	d, ec := explainEnv(t, out)
	if code != 0 || ec != "" || d.Source != planview.SourceDecisionRecord || d.Pipeline.Name != "run1" || d.Jobs[0].ID != "build" {
		t.Fatalf("exit %d %s %s", code, out, errs)
	}
}

func TestExplainUnknownJobAndUsage(t *testing.T) {
	repo, _ := placementRepo(t)
	resetPlanFlags()
	code, out, _ := capture(t, func() int { return runExplain([]string{"nope", "--json", "--repo", repo}) })
	if _, ec := explainEnv(t, out); code != 1 || ec != "E692" || !strings.Contains(out, "build, test") {
		t.Fatalf("unknown job: exit %d\n%s", code, out)
	}
	resetPlanFlags()
	if code, _, _ = capture(t, func() int { return runExplain(nil) }); code != 1 {
		t.Fatalf("no target: exit %d", code)
	}
}

func TestExplainRegistered(t *testing.T) {
	e, ok := v15Subcommands["explain"]
	if !ok || !e.V15Only {
		t.Fatal("explain is not a V15-only subcommand")
	}
}
