package plan

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func job(deps ...string) model.Job {
	return model.Job{Kind: "test", Run: []string{"true"}, Deps: deps}
}

func cfgOf(jobs map[string]model.Job) model.PipelineConfig {
	return model.PipelineConfig{Version: 1, Jobs: jobs}
}

func planErr(t *testing.T, in Input, code string) *Error {
	t.Helper()
	_, err := Build(in)
	var pe *Error
	if !errors.As(err, &pe) || pe.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
	return pe
}

func TestCycleAndDangling(t *testing.T) {
	w := linuxWorld()
	e := planErr(t, Input{World: w, Config: cfgOf(map[string]model.Job{"a": job("b"), "b": job("c"), "c": job("a"), "d": job()})}, CodeCycle)
	if !reflect.DeepEqual(e.Jobs, []string{"a", "b", "c"}) || !strings.Contains(e.Message, "a -> b -> c -> a") {
		t.Errorf("cycle error = %+v", e)
	}
	e = planErr(t, Input{World: w, Config: cfgOf(map[string]model.Job{"a": job("ghost")})}, CodeCycle)
	if !reflect.DeepEqual(e.Jobs, []string{"a", "ghost"}) {
		t.Errorf("dangling error = %+v", e)
	}
	_ = planErr(t, Input{World: w, Config: cfgOf(map[string]model.Job{"a": job("a")})}, CodeCycle)
}

func TestRequiredDependsOnAllowFailure(t *testing.T) {
	soft := job()
	soft.AllowFailure = true
	soft.Required = bptr(false)
	cfg := cfgOf(map[string]model.Job{"soft": soft, "hard": job("soft")})
	_ = planErr(t, Input{World: linuxWorld(), Config: cfg}, CodeRequiredAllowFailure)
	hard := job("soft")
	hard.Required = bptr(false)
	cfg.Jobs["hard"] = hard
	mustBuild(t, Input{World: linuxWorld(), Config: cfg})
}

func TestUnknownNames(t *testing.T) {
	cfg := cfgOf(map[string]model.Job{"a": job()})
	cfg.Pipelines = map[string]model.PipelineDef{"p": {Triggers: []model.Trigger{"push"}, Jobs: []string{"missing"}}}
	_ = planErr(t, Input{World: linuxWorld(), Config: cfg, Pipeline: "nope"}, CodeUnknownName)
	_ = planErr(t, Input{World: linuxWorld(), Config: cfg, Pipeline: "p"}, CodeUnknownName)
	cfg.Pipelines = nil
	cfg.Policy = map[string]any{"fail_fast": "sideways"}
	_ = planErr(t, Input{World: linuxWorld(), Config: cfg}, CodeUnknownName)
}

func TestLayersAndPriority(t *testing.T) {
	hi, lo := job(), job()
	hi.Priority, lo.Priority = "high", "low"
	cfg := cfgOf(map[string]model.Job{"z-hi": hi, "a-lo": lo, "mid": job(), "top": job("z-hi", "mid")})
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg})
	want := [][]string{{"z-hi", "mid", "a-lo"}, {"top"}}
	if !reflect.DeepEqual(p.Layers, want) {
		t.Errorf("layers = %v, want %v", p.Layers, want)
	}
	if got := find(p, "z-hi").Dependants; !reflect.DeepEqual(got, []string{"top"}) {
		t.Errorf("dependants = %v", got)
	}
	if find(p, "top").Layer != 1 {
		t.Error("layer index wrong")
	}
}

func TestFailFastScopes(t *testing.T) {
	soft := job()
	soft.AllowFailure, soft.Required = true, bptr(false)
	cfg := cfgOf(map[string]model.Job{"a": job(), "b": job("a"), "c": job("b"), "d": job(), "soft": soft})
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg})
	if ids, r := p.CancelSet("a"); !reflect.DeepEqual(ids, []string{"b", "c"}) || r != "dep.failed" {
		t.Errorf("dependants scope: %v %s", ids, r)
	}
	if ids, _ := p.CancelSet("soft"); len(ids) != 0 {
		t.Errorf("allow_failure cancelled %v", ids)
	}
	cfg.Policy = map[string]any{"fail_fast": "pipeline"}
	p = mustBuild(t, Input{World: linuxWorld(), Config: cfg})
	if ids, r := p.CancelSet("d"); len(ids) != 4 || r != "fail_fast.cancelled" || p.FailFast != "pipeline" {
		t.Errorf("pipeline scope: %v %s", ids, r)
	}
}

func TestPipelineSelection(t *testing.T) {
	drill := job()
	drill.Kind = "drill"
	cfg := cfgOf(map[string]model.Job{"build": job(), "test": job("build"), "restore": drill, "lint": job()})
	cfg.Pipelines = map[string]model.PipelineDef{
		"ci":      {Triggers: []model.Trigger{"push"}, Jobs: []string{"test"}},
		"nightly": {Triggers: []model.Trigger{"schedule", "push"}, Jobs: []string{"lint"}},
		"drills":  {Triggers: []model.Trigger{"manual"}, Jobs: []string{"restore"}},
	}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg})
	if p.Pipeline.Kind != "default" || find(p, "restore") != nil || skippedReason(p, "restore") != "pipeline.excluded" || len(p.Jobs) != 3 {
		t.Errorf("default pipeline: %+v skipped %v", p.Pipeline, p.Skipped)
	}
	p = mustBuild(t, Input{World: linuxWorld(), Config: cfg, Pipeline: "ci"})
	if p.Pipeline.Name != "ci" || p.Pipeline.Kind != "named" || len(p.Jobs) != 2 || skippedReason(p, "lint") != "pipeline.excluded" {
		t.Errorf("named pipeline: %+v jobs %d", p.Pipeline, len(p.Jobs))
	}
	p = mustBuild(t, Input{World: linuxWorld(), Config: cfg, Trigger: "push"})
	if p.Pipeline.Name != "ci+nightly" || len(p.Jobs) != 3 || p.Trigger != "push" {
		t.Errorf("trigger pipeline: %+v jobs %d", p.Pipeline, len(p.Jobs))
	}
	p = mustBuild(t, Input{World: linuxWorld(), Config: cfg, Pipeline: "drills"})
	if p.Pipeline.Kind != "drill" || len(p.Jobs) != 1 {
		t.Errorf("drill pipeline: %+v", p.Pipeline)
	}
	p = mustBuild(t, Input{World: linuxWorld(), Config: cfg, Trigger: "api"})
	if p.Pipeline.Name != "default" {
		t.Errorf("a trigger no pipeline lists must plan the default pipeline, got %s", p.Pipeline.Name)
	}
}

func TestWhenConditions(t *testing.T) {
	on := func(w model.WhenConfig) model.Job { j := job(); j.Required = bptr(false); j.When = &w; return j }
	cfg := cfgOf(map[string]model.Job{
		"branch": on(model.WhenConfig{Branches: []string{"release/*"}}),
		"paths":  on(model.WhenConfig{Paths: []string{"docs/**"}}),
		"trig":   on(model.WhenConfig{Triggers: []model.Trigger{"tag"}}),
		"env":    on(model.WhenConfig{EnvPresent: []string{"TOKEN"}}),
		"child":  func() model.Job { j := job("branch"); j.Required = bptr(false); return j }(),
	})
	rev := Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"src/a.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	for id, want := range map[string]model.Reason{"branch": "when.branch", "paths": "when.paths", "trig": "when.trigger", "child": "dep.failed"} {
		if got := skippedReason(p, id); got != want {
			t.Errorf("%s skipped with %q, want %q", id, got, want)
		}
	}
	if j := find(p, "env"); j == nil || j.State != StateSkip || j.Blocked != "when.env_missing" {
		t.Errorf("env job = %+v", j)
	}
	rev.Branch, rev.Env = "release/1", []string{"TOKEN"}
	rev.Changed = []string{"docs/a.md"}
	p = mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev, Trigger: "tag"})
	if len(p.Jobs) != 5 || find(p, "env").State != StatePlanned {
		t.Errorf("all conditions met: %d jobs, env %s", len(p.Jobs), find(p, "env").State)
	}
}

func TestGlobMatch(t *testing.T) {
	for _, c := range []struct {
		pat, val string
		want     bool
	}{
		{"docs/**", "docs/a/b.md", true}, {"docs/**", "src/a.md", false}, {"**/*.go", "a/b/c.go", true},
		{"*.go", "a/b.go", false}, {"src", "src/x/y", true}, {"src", "srcx/y", false},
		{"release/*", "release/1", true}, {"release/*", "release/1/2", false}, {"a/?.txt", "a/b.txt", true},
	} {
		if got := globMatch(c.pat, c.val); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v", c.pat, c.val, got)
		}
	}
}

func TestUnplaceableDockerAndServices(t *testing.T) {
	c := job()
	c.Container = &model.ContainerConfig{Image: "img@sha256:" + strings.Repeat("0", 64)}
	svc := job()
	svc.NeedsServices = []string{"db"}
	cfg := cfgOf(map[string]model.Job{"boxed": c, "svc": svc})
	cfg.Services = map[string]model.Service{"db": {Component: "postgres"}}
	w := linuxWorld()
	w.Docker = false
	p := mustBuild(t, Input{World: w, Config: cfg})
	for _, id := range []string{"boxed", "svc"} {
		if j := find(p, id); j.Blocked != "unplaceable.docker" {
			t.Errorf("%s = %s", id, j.Blocked)
		}
	}
	w = linuxWorld()
	w.ServicesKnown = true
	if j := find(mustBuild(t, Input{World: w, Config: cfg}), "svc"); j.Blocked != "unplaceable.service" || !strings.Contains(j.Detail, "db") {
		t.Errorf("svc = %+v", j)
	}
	w.Runners = nil
	m := job()
	m.Matrix = &model.JobMatrix{Platform: []any{"darwin/arm64"}}
	cfg = cfgOf(map[string]model.Job{"mac": m})
	cfg.Support = []string{"linux/amd64"}
	if j := find(mustBuild(t, Input{World: w, Config: cfg}), "mac@darwin/arm64"); j == nil || j.Blocked != "unplaceable.platform" || !strings.Contains(j.Detail, "support") {
		t.Errorf("a matrix outside the support policy must stay as one unplaceable entry, got %+v", j)
	}
	cfg.Support = []string{"linux/amd64", "darwin/arm64"}
	if j := find(mustBuild(t, Input{World: w, Config: cfg}), "mac@darwin/arm64"); j.Blocked != "unplaceable.platform" {
		t.Errorf("mac = %+v", j)
	}
	w.Runners = []string{"darwin/arm64"}
	if j := find(mustBuild(t, Input{World: w, Config: cfg}), "mac@darwin/arm64"); j.State != StatePlanned {
		t.Errorf("registered runner should place it, got %s", j.State)
	}
}

func TestRetryNormalisation(t *testing.T) {
	big, no := job(), job()
	big.Retry = &model.RetryConfig{InfraMax: 99}
	no.Idempotent = bptr(false)
	cfg := cfgOf(map[string]model.Job{"big": big, "no": no, "dflt": job()})
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg})
	for id, want := range map[string]int{"big": MaxInfraRetries, "no": 0, "dflt": 1} {
		if got := find(p, id).Job.Retry.InfraMax; got != want {
			t.Errorf("%s infra_max = %d, want %d", id, got, want)
		}
	}
	if j := find(p, "dflt").Job; j.Timeout != "10m" || j.Path != "deep" || *j.Required != true {
		t.Errorf("defaults not applied: %+v", j)
	}
}

func TestTransitiveRequiredAllowFailure(t *testing.T) {
	soft := job()
	soft.AllowFailure, soft.Required = true, bptr(false)
	mid := job("c")
	mid.Required = bptr(false)
	cfg := cfgOf(map[string]model.Job{"a": job("b"), "b": mid, "c": soft})
	e := planErr(t, Input{World: linuxWorld(), Config: cfg}, CodeRequiredAllowFailure)
	if !reflect.DeepEqual(e.Jobs, []string{"a", "b", "c"}) || !strings.Contains(e.Message, "via b") {
		t.Errorf("error = %+v", e)
	}
	a := job("b")
	a.Required = bptr(false)
	cfg.Jobs["a"] = a
	mustBuild(t, Input{World: linuxWorld(), Config: cfg})
}

func TestUnknownTrigger(t *testing.T) {
	cfg := cfgOf(map[string]model.Job{"a": job()})
	_ = planErr(t, Input{World: linuxWorld(), Config: cfg, Trigger: "pushh"}, CodeUnknownName)
}

// Epic D28 for named and trigger-selected pipelines: a default-branch or
// release plan keeps every required deep job outside the selected pipeline.
func TestPipelineSelectionKeepsRequiredDeep(t *testing.T) {
	lint, test := job(), job()
	lint.Path, lint.Workdir, test.Workdir = "fast", "web", "svc"
	cfg := cfgOf(map[string]model.Job{"lint": lint, "test": test})
	cfg.Pipelines = map[string]model.PipelineDef{"quick": {Triggers: []model.Trigger{"push"}, Jobs: []string{"lint"}}}
	rev := Revision{Branch: "main", DefaultBranch: true, BaseKnown: true, Changed: []string{"web/a.ts"}}
	for name, in := range map[string]Input{
		"named":   {World: linuxWorld(), Config: cfg, Revision: rev, Pipeline: "quick"},
		"trigger": {World: linuxWorld(), Config: cfg, Revision: rev, Trigger: "push"},
	} {
		p := mustBuild(t, in)
		if j := find(p, "test"); j == nil || j.Reason != "selection.full_required" {
			t.Errorf("%s: required deep job test = %+v", name, j)
		}
	}
	rev.DefaultBranch = false
	if p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev, Pipeline: "quick"}); find(p, "test") != nil {
		t.Error("a feature-branch plan must still honour the named pipeline")
	}
	rev.DefaultBranch, rev.Release = false, true
	if p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev, Pipeline: "quick"}); find(p, "test") == nil {
		t.Error("a release plan dropped a required deep job")
	}
}

func TestEmptyDiffPlansEverything(t *testing.T) {
	cfg := cfgOf(map[string]model.Job{"a": job(), "b": job()})
	rev := Revision{Branch: "main", DefaultBranch: true, BaseKnown: true}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	if len(p.Jobs) != 2 || p.Selection.Mode != "full" {
		t.Errorf("empty diff planned %d jobs, selection %+v", len(p.Jobs), p.Selection)
	}
}

func TestSelectionDigestCoversForcedJobs(t *testing.T) {
	defer SetSelector(nil)
	SetSelector(noJobs)
	lint := job()
	lint.Path = "fast"
	cfg := cfgOf(map[string]model.Job{"lint": lint, "test": job()})
	rev := Revision{Branch: "main", DefaultBranch: true, BaseKnown: true, Changed: []string{"x.go"}}
	p := mustBuild(t, Input{World: linuxWorld(), Config: cfg, Revision: rev})
	want := selectionDigest(rev.Changed, map[string]bool{"test": true})
	if p.Selection.Digest != want || p.Selection.Digest == selectionDigest(rev.Changed, map[string]bool{}) {
		t.Errorf("digest %s does not cover the forced job set", p.Selection.Digest)
	}
}
