package main

// Purpose: the truth check is linked into the V15 engine (D-1240): a repo
// config with checks.truth.rules plans a required `truth` job whose command is
// this binary's own `truth check`, and that command runs the claim rules.
// Constraints: the repo is a temp tree; no network, no host tools.

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

const truthRules = `version: 1
rules:
  - id: no-marker
    kind: banned-literal
    literal: SEKRET_MARKER
    message: remove the marker
`

func truthRepo(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range map[string]string{".github/truth/org.yaml": truthRules, "README.md": body} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func truthConfig(jobs map[string]model.Job) model.PipelineConfig {
	return model.PipelineConfig{Version: 1, Jobs: jobs,
		Checks: map[string]model.CheckDef{"truth": {Rules: []string{".github/truth/org.yaml"}}}}
}

func TestPlanTruthFromRules(t *testing.T) {
	root := truthRepo(t, "clean\n")
	cfg := truthConfig(nil)
	facts, checks, err := planChecks(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got *plan.Check
	for i := range checks {
		if checks[i].ID == "truth" {
			got = &checks[i]
		}
	}
	if got == nil {
		t.Fatalf("checks.truth.rules plans no truth check: %+v", checks)
	}
	want := []string{"nself-ci", "truth", "check", "--root", ".", ".github/truth/org.yaml"}
	if !reflect.DeepEqual(got.Argv, want) || !got.Required || !got.Substantive {
		t.Fatalf("truth check: %+v", got)
	}
	p, err := plan.Build(plan.Input{Config: cfg, Facts: facts, Checks: checks,
		World: plan.World{Platform: "linux/amd64", Tools: []string{"nself-ci"}, CPU: 4, MemMB: 8192}, Revision: plan.Revision{DefaultBranch: true}})
	if err != nil {
		t.Fatal(err)
	}
	if j := jobNamed(t, p, "truth"); j.Job.Env["NSELF_V15"] != "1" {
		t.Fatalf("truth job env: %+v", j.Job.Env)
	}
	if j := jobNamed(t, p, "truth"); j.State != plan.StatePlanned {
		t.Fatalf("truth job: %+v", j)
	}
}

func TestPlanTruthNeedsRules(t *testing.T) {
	_, checks, err := planChecks(truthRepo(t, "clean\n"), model.PipelineConfig{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		if c.ID == "truth" {
			t.Fatalf("a truth check without checks.truth.rules: %+v", c)
		}
	}
}

// A job that already runs the check (the plugins repo runs it from source) is
// not planned a second time.
func TestPlanTruthSkippedWhenAJobRunsIt(t *testing.T) {
	cfg := truthConfig(map[string]model.Job{"gate": {Kind: "static", Run: []string{"go", "run", "run.go", "truth", "--root", "."}}})
	_, checks, err := planChecks(truthRepo(t, "clean\n"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		if c.ID == "truth" {
			t.Fatalf("truth planned beside a job that runs it: %+v", c)
		}
	}
}

func TestPlanTruthWithJobsAlsoPlansIt(t *testing.T) {
	cfg := truthConfig(map[string]model.Job{"unit": {Kind: "test", Run: []string{"go", "test", "./..."}}})
	_, checks, err := planChecks(truthRepo(t, "clean\n"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range checks {
		if c.ID == "truth" {
			return
		}
	}
	t.Fatalf("jobs declared, checks.truth.rules set, no truth check: %+v", checks)
}

func runTruth(t *testing.T, args ...string) (int, string) {
	t.Helper()
	entry, ok := v15Subcommands["truth"]
	if !ok {
		t.Fatal("the truth subcommand is not registered for V15")
	}
	if !entry.V15Only || !entry.Hidden {
		t.Fatalf("truth must be V15-only and hidden: %+v", entry)
	}
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code := entry.Handler(args)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return code, buf.String()
}

func TestTruthSubcommandPassAndFail(t *testing.T) {
	clean := truthRepo(t, "clean\n")
	code, out := runTruth(t, "check", "--root", clean, "--rules-root", clean, ".github/truth/org.yaml")
	if code != 0 || !strings.Contains(out, "PASS truth.banned-literal") {
		t.Fatalf("clean tree: code %d\n%s", code, out)
	}
	dirty := truthRepo(t, "has SEKRET_MARKER inside\n")
	code, out = runTruth(t, "check", "--root", dirty, "--rules-root", dirty, ".github/truth/org.yaml")
	if code != 1 || !strings.Contains(out, "FAIL truth.banned-literal") {
		t.Fatalf("dirty tree: code %d\n%s", code, out)
	}
}
