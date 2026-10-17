package plan

// Shared fixtures for the planner tests: synthetic worlds, configs and the
// golden-file comparison (UPDATE_GOLDEN=1 rewrites goldens).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func linuxWorld(tools ...string) World {
	return World{Platform: "linux/amd64", Tools: tools, Docker: true, CPU: 8, MemMB: 16384}
}

func loadConfig(t *testing.T, name string) model.PipelineConfig {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var cfg model.PipelineConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// checksFor detects the fixture and plans its catalogue checks the way
// cmd/plan.go does.
func checksFor(t *testing.T, dir string) ([]detect.Fact, []Check) {
	t.Helper()
	reg := gates.DefaultRegistry()
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(filepath.Join("testdata", dir)), reg.Has)
	if err != nil {
		t.Fatal(err)
	}
	var out []Check
	for _, c := range reg.Plan(facts, gates.Options{}) {
		argv := make([]string, len(c.Def.Argv))
		for i, a := range c.Def.Argv {
			argv[i] = strings.ReplaceAll(a, "{target}", c.Target)
		}
		out = append(out, Check{ID: c.Def.ID, Kind: c.Def.Kind, Tool: c.Def.Tool, Workdir: c.Workdir, Target: c.Target,
			Argv: argv, Required: c.Required, Substantive: c.Def.Substantive, Path: c.Def.Path})
	}
	return facts, out
}

func mustBuild(t *testing.T, in Input) *Plan {
	t.Helper()
	p, err := Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return p
}

func envelope(t *testing.T, p *Plan) []byte {
	t.Helper()
	b, err := output.Data("ci plan", p, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from the plan output:\n%s", name, got)
	}
}

func find(p *Plan, id string) *PlannedJob {
	for i := range p.Jobs {
		if p.Jobs[i].ID == id {
			return &p.Jobs[i]
		}
	}
	return nil
}

func skippedReason(p *Plan, id string) model.Reason {
	for _, s := range p.Skipped {
		if s.Job == id {
			return s.Reason
		}
	}
	return ""
}

func bptr(b bool) *bool { return &b }
