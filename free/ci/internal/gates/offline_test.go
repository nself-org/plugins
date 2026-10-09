//go:build offline

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

func TestOfflineGoFixture(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	// sh and ps are the executor's host process utilities, not check tools.
	for _, name := range []string{"go", "gofmt", "git", "gitleaks", "sh", "ps"} {
		tool, err := findTool(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(tool, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("GOFLAGS", "-mod=vendor")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	writeFixture(t, root, "go.mod", "module example.org/fixture\n\ngo 1.22\n")
	writeFixture(t, root, "main.go", "package fixture\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFixture(t, root, "main_test.go", "package fixture\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1, 2) != 3 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	r := DefaultRegistry()
	facts, err := detect.DefaultRegistry().Detect(os.DirFS(root), r.Has)
	if err != nil {
		t.Fatal(err)
	}
	floor := 50
	cfg := model.PipelineConfig{Checks: map[string]model.CheckDef{"go.coverage": {CoverageFloor: &floor}}}
	plan := r.Plan(facts, Options{Config: cfg})
	runner := Runner{Executor: &exec.Executor{}, Home: t.TempDir()}
	checks := make([]model.Check, 0, len(plan))
	seen := map[string]bool{}
	for _, p := range plan {
		c := runner.Run(context.Background(), root, p, cfg)
		checks = append(checks, c)
		seen[c.ID] = true
		if c.ID == "go.coverage" && c.Result != "pass" {
			t.Fatalf("coverage: %+v", c)
		}
		if c.Required && c.Result != "pass" {
			t.Fatalf("%s: %+v", c.ID, c)
		}
	}
	for _, id := range []string{"go.fmt", "go.vet", "go.test", "go.coverage", "secrets.gitleaks", "go.govulncheck", "security.trivy.fs"} {
		if !seen[id] {
			t.Fatalf("missing %s", id)
		}
	}
	for _, c := range checks {
		if (c.ID == "go.govulncheck" || c.ID == "security.trivy.fs") && (c.Result != "skip" || c.Reason != "advisory.tool_missing") {
			t.Fatal(c)
		}
	}
	if model.Verdict(checks) != "pass" {
		t.Fatal(checks)
	}
}

func TestMissingGitleaks(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.org/fixture\n\ngo 1.22\n")
	bin := t.TempDir()
	for _, name := range []string{"go", "gofmt", "git"} {
		path, err := findTool(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	d, _ := DefaultRegistry().Get("secrets.gitleaks")
	c := (Runner{Executor: &exec.Executor{}, Home: t.TempDir()}).Run(context.Background(), root, PlannedCheck{Def: d, Required: true}, model.PipelineConfig{})
	if c.Result != "error" || !strings.Contains(c.Excerpt, "E614") || !strings.Contains(c.Excerpt, "Install gitleaks") {
		t.Fatal(c)
	}
}

func findTool(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if s, e := os.Stat(p); e == nil && !s.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func writeFixture(t *testing.T, root, name, data string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
