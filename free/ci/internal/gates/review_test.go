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

func nodeFixture(t *testing.T, manifest string) (string, Runner) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	return root, Runner{Executor: &exec.Executor{}, Home: t.TempDir()}
}

func TestReviewUnsupportedNodeManagerFailsClosed(t *testing.T) {
	root, runner := nodeFixture(t, `{"packageManager":"unknown@1","scripts":{"test":"true"}}`)
	d, _ := DefaultRegistry().Get("node.test")
	c := runner.Run(context.Background(), root, PlannedCheck{Def: d, Required: true, Workdir: "."}, model.PipelineConfig{})
	if c.Result != "error" || !c.Required || !strings.Contains(c.Excerpt, "E614") {
		t.Fatal(c)
	}
}

func TestReviewPnpmWithoutNpm(t *testing.T) {
	root, runner := nodeFixture(t, `{"packageManager":"pnpm@9","scripts":{"test":"true"}}`)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pnpm"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/ps", filepath.Join(bin, "ps")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	d, _ := DefaultRegistry().Get("node.test")
	c := runner.Run(context.Background(), root, PlannedCheck{Def: d, Required: true, Workdir: "."}, model.PipelineConfig{})
	if c.Tool != "pnpm" || c.Result != "pass" {
		t.Fatal(c)
	}
}

func TestReviewTrivyFindingContainingOffline(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	body := `{"Results":[{"Vulnerabilities":[{"Title":"offline signing flaw"}]}]}`
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 1; else echo '" + body + "'; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "trivy"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/ps", filepath.Join(bin, "ps")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	d, _ := DefaultRegistry().Get("security.trivy.fs")
	c := (Runner{Executor: &exec.Executor{}, Home: t.TempDir()}).Run(context.Background(), root, PlannedCheck{Def: d, Workdir: "."}, model.PipelineConfig{})
	if c.Result != "fail" || c.Findings != 1 {
		t.Fatal(c)
	}
}

func TestReviewRequiredCannotBeDowngraded(t *testing.T) {
	f := false
	for _, tc := range []struct {
		name string
		o    Options
	}{{"container", Options{Container: true}}, {"release", Options{Kind: "release"}}, {"strict", Options{Config: model.PipelineConfig{Presets: []model.Preset{"strict"}}}}} {
		t.Run(tc.name, func(t *testing.T) {
			tc.o.Config.Checks = map[string]model.CheckDef{"go.govulncheck": {Required: &f}}
			p := DefaultRegistry().Plan([]detect.Fact{{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain}}, tc.o)
			for _, c := range p {
				if c.Def.ID == "go.govulncheck" {
					if !c.Required {
						t.Fatal(c)
					}
					return
				}
			}
			t.Fatal("missing check")
		})
	}
}

func TestReviewNestedDockerfileTarget(t *testing.T) {
	p := DefaultRegistry().Plan([]detect.Fact{{Kind: "container", Name: "dockerfile", Path: "app/Dockerfile", Confidence: detect.Certain}}, Options{})
	for _, c := range p {
		if c.Def.ID == "container.hadolint" || c.Def.ID == "container.trivy.config" {
			if c.Workdir != "app" || c.Target != "Dockerfile" {
				t.Fatal(c)
			}
		}
	}
}

func TestReviewImpliedJobNameCollision(t *testing.T) {
	r := NewRegistry()
	d := def("fixture", "quality", "go", []string{"go", "version"}, false, true, "fast", "install Go")
	d.ImpliedBy = "checks.fixture.rules"
	if err := r.Register(d); err != nil {
		t.Fatal(err)
	}
	c := model.PipelineConfig{Checks: map[string]model.CheckDef{"fixture": {Rules: []string{"one"}}}, Jobs: map[string]model.Job{"fixture": {Run: []string{"echo unrelated"}}}}
	jobs := r.ImpliedJobs(c)
	for name, j := range jobs {
		if name != "fixture" && len(j.Run) == 1 && j.Run[0] == "fixture" && j.Required != nil && *j.Required {
			return
		}
	}
	t.Fatal(jobs)
}
