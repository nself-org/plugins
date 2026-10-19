package android

import (
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

func TestResolveSynthesizes(t *testing.T) {
	cfg := model.PipelineConfig{Version: 1, Jobs: map[string]model.Job{"unit": {Kind: "test", Run: []string{"true"}}}}
	l, err := Resolve(Input{Config: cfg, Workdir: "android", Timeout: time.Minute, Required: []string{"unit", "lint", "unit", "android"}})
	if err != nil || !l.Synthesized || l.Pipeline != PipelineName {
		t.Fatalf("lane %+v err %v", l, err)
	}
	if got := l.Config.Pipelines[PipelineName].Jobs; len(got) != 3 || got[0] != "lint" || got[1] != "unit" || got[2] != JobName {
		t.Fatalf("pipeline jobs %v", got)
	}
	j := l.Config.Jobs[JobName]
	if j.Kind != "release" || len(j.Deps) != 2 || j.Workdir != "android" || j.Timeout != "60s" {
		t.Fatalf("job %+v", j)
	}
	if _, ok := cfg.Jobs[JobName]; ok || len(cfg.Pipelines) != 0 {
		t.Fatal("Resolve changed its input")
	}
}

func TestResolveUsesConfiguredJob(t *testing.T) {
	own := model.Job{Kind: "release", Run: []string{"make", "apk"}, Deps: []string{"unit"}}
	cfg := model.PipelineConfig{Jobs: map[string]model.Job{JobName: own, "unit": {Kind: "test", Run: []string{"true"}}}}
	l, err := Resolve(Input{Config: cfg, Workdir: "x", Timeout: time.Minute, Required: []string{"unit"}})
	if err != nil || l.Synthesized || l.Pipeline != PipelineName || l.Config.Jobs[JobName].Run[0] != "make" {
		t.Fatalf("lane %+v err %v", l, err)
	}
	if got := l.Config.Pipelines[PipelineName].Jobs; len(got) != 1 || got[0] != JobName {
		t.Fatalf("pipeline jobs %v", got)
	}
	cfg.Pipelines = map[string]model.PipelineDef{PipelineName: {Triggers: []model.Trigger{"manual"}, Jobs: []string{"unit", JobName}}}
	if l, err = Resolve(Input{Config: cfg}); err != nil || len(l.Config.Pipelines[PipelineName].Jobs) != 2 {
		t.Fatalf("the project's own pipeline was replaced: %+v %v", l, err)
	}
}

func TestResolveConflicts(t *testing.T) {
	notRelease := model.PipelineConfig{Jobs: map[string]model.Job{JobName: {Kind: "test", Run: []string{"true"}}}}
	if _, err := Resolve(Input{Config: notRelease}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a test job named android: %v", err)
	}
	orphan := model.PipelineConfig{Pipelines: map[string]model.PipelineDef{PipelineName: {Jobs: []string{"unit"}}}}
	if _, err := Resolve(Input{Config: orphan}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a pipeline without the job: %v", err)
	}
}

func TestRequiredJobs(t *testing.T) {
	pj := func(name string, kind model.JobKind, req *bool) plan.PlannedJob {
		return plan.PlannedJob{Name: name, Job: model.Job{Kind: kind, Required: req}}
	}
	p := &plan.Plan{Jobs: []plan.PlannedJob{pj("b", "test", yes()), pj("a", "build", yes()), pj("a", "build", yes()), pj("adv", "lint", no()),
		pj("rel", "release", yes()), pj("dr", "drill", yes()), pj("nil", "test", nil)}}
	if got := RequiredJobs(p); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("required %v", got)
	}
}
