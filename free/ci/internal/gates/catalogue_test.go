package gates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestDetectorBindings(t *testing.T) {
	r := DefaultRegistry()
	for _, id := range []string{"go.test", "node.test", "rust.test", "flutter.test", "container.compose", "container.docker", "inputs.lockfiles"} {
		if !r.Has(id) {
			t.Errorf("missing %s", id)
		}
	}
	if _, err := detect.DefaultRegistry().Detect(os.DirFS("."), r.Has); err != nil {
		t.Fatal(err)
	}
}

func TestImpliedJob(t *testing.T) {
	r := NewRegistry()
	d := def("fixture", "quality", "go", []string{"go", "version"}, false, true, "fast", "install Go")
	d.ImpliedBy = "checks.fixture.rules"
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	if len(r.ImpliedJobs(model.PipelineConfig{})) != 0 {
		t.Fatal("unconfigured fixture planned")
	}
	c := model.PipelineConfig{Checks: map[string]model.CheckDef{"fixture": {Rules: []string{"one"}}}}
	if j, ok := r.ImpliedJobs(c)["fixture"]; !ok || j.Required == nil || !*j.Required {
		t.Fatalf("implied job: %+v", j)
	}
	c.Jobs = map[string]model.Job{"existing": {Run: []string{"fixture"}}}
	if len(r.ImpliedJobs(c)) != 1 {
		t.Fatal("duplicate implied job")
	}
}

func TestRequiredResolution(t *testing.T) {
	d := DefaultRegistry()
	facts := []detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain}}
	for _, tc := range []struct {
		name string
		o    Options
		want bool
	}{{"local", Options{}, false}, {"container", Options{Container: true}, true}, {"release", Options{Kind: "release"}, true}, {"strict", Options{Config: model.PipelineConfig{Presets: []model.Preset{"strict"}}}, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p := d.Plan(facts, tc.o)
			found := false
			for _, c := range p {
				if c.Def.ID == "go.govulncheck" {
					found = true
					if c.Required != tc.want {
						t.Fatal(c.Required)
					}
				}
			}
			if !found {
				t.Fatal("govulncheck missing")
			}
		})
	}
}

func TestMissingRequiredAndAdvisory(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	r := Runner{Executor: &exec.Executor{}, Home: t.TempDir()}
	for _, required := range []bool{true, false} {
		p := PlannedCheck{Def: DefaultRegistry().All()[0], Required: required}
		p.Def.Tool = "absent-tool"
		c := r.Run(context.Background(), t.TempDir(), p, model.PipelineConfig{})
		if required && (c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "Install")) {
			t.Fatalf("required: %+v", c)
		}
		if !required && (c.Result != "skip" || c.Reason != "advisory.tool_missing") {
			t.Fatalf("advisory: %+v", c)
		}
	}
}

func TestCoverageFloor(t *testing.T) {
	floor := 80
	c := model.Check{Result: "pass"}
	p := PlannedCheck{Def: CheckDef{ID: "go.coverage"}}
	applyCoverage(&c, "", p, model.PipelineConfig{Checks: map[string]model.CheckDef{"go.coverage": {CoverageFloor: &floor}}}, invocation{output: "total: (statements) 70.0%"})
	if c.Result != "fail" {
		t.Fatal(c)
	}
	node := model.Check{Result: "pass"}
	np := PlannedCheck{Def: CheckDef{ID: "node.coverage"}}
	applyCoverage(&node, "", np, model.PipelineConfig{Checks: map[string]model.CheckDef{"node.coverage": {CoverageFloor: &floor}}}, invocation{output: "TN:\nSF:index.js\nLF:10\nLH:7\nend_of_record\n"})
	if node.Result != "fail" {
		t.Fatal(node)
	}
}

func TestAdvisoryFindings(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	for name, body := range map[string]string{
		"govulncheck": "{\"finding\":{\"osv\":\"GO-TEST\"}}",
		"trivy":       "{\"Results\":[{\"Vulnerabilities\":[{\"VulnerabilityID\":\"CVE-TEST\"}]}]}",
		"hadolint":    "[{\"code\":\"DL3000\",\"message\":\"bad Dockerfile\"}]",
	} {
		script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '1.0\\n'; else printf '%s\\n' '" + body + "'; fi\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := Runner{Executor: &exec.Executor{}, Home: t.TempDir()}
	reg := DefaultRegistry()
	for _, id := range []string{"go.govulncheck", "security.trivy.fs", "container.hadolint"} {
		d, _ := reg.Get(id)
		for _, required := range []bool{false, true} {
			c := r.Run(context.Background(), root, PlannedCheck{Def: d, Required: required, Workdir: ".", Target: "Dockerfile"}, model.PipelineConfig{})
			if c.Result != "fail" || c.Findings != 1 || c.Required != required {
				t.Fatalf("%s required=%v: %+v", id, required, c)
			}
		}
	}
}

func TestTrivyOffline(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1.0; else echo 'database download failed: offline'; exit 1; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "trivy"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	d, _ := DefaultRegistry().Get("security.trivy.fs")
	r := Runner{Executor: &exec.Executor{}, Home: t.TempDir()}
	advisory := r.Run(context.Background(), root, PlannedCheck{Def: d}, model.PipelineConfig{})
	if advisory.Result != "skip" || advisory.Reason != "advisory.data_offline" {
		t.Fatal(advisory)
	}
	required := r.Run(context.Background(), root, PlannedCheck{Def: d, Required: true}, model.PipelineConfig{})
	if required.Result != "fail" {
		t.Fatal(required)
	}
}
