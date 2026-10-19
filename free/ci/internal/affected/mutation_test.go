package affected

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/gates"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// leafSelectionHolds is the fixture assertion the mutation test breaks: a
// change to leaf must select leaf, mid and app, and nothing else.
func leafSelectionHolds(g *Graph) error {
	in := plan.SelectionInput{Changed: []string{"leaf/leaf.go"}, BaseKnown: true, Jobs: fixtureJobs()}
	sel, jobs := selectWith(g, true, in)
	if want := []string{"app.test", "leaf.test", "mid.test"}; sel.Mode != "affected" || !reflect.DeepEqual(jobs, want) {
		return &selectionError{got: jobs}
	}
	return nil
}

type selectionError struct{ got []string }

func (e *selectionError) Error() string { return "leaf change selected " + strings.Join(e.got, ",") }

// TestReverseEdgeMutation removes each reverse edge of the leaf chain in
// turn: the fixture assertion must fail every time, because a needed job
// would be skipped.
func TestReverseEdgeMutation(t *testing.T) {
	g, err := Build(os.DirFS("testdata/monorepo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := leafSelectionHolds(g); err != nil {
		t.Fatalf("unmutated graph: %v", err)
	}
	for _, e := range [][2]string{{"mid", "leaf"}, {"app", "mid"}} {
		m := g.withoutEdge(e[0], e[1])
		if len(m.edges) != len(g.edges)-1 {
			t.Fatalf("mutation of %v removed %d edges", e, len(g.edges)-len(m.edges))
		}
		if leafSelectionHolds(m) == nil {
			t.Errorf("dropping the edge %s -> %s went unnoticed", e[0], e[1])
		}
	}
}

// TestEvidenceSelection plans the fixture end to end through the real
// planner with the affected detector and selector installed and checks the
// selection that evidence copies from the plan.
func TestEvidenceSelection(t *testing.T) {
	dir := "testdata/monorepo"
	reg := detect.DefaultRegistry()
	if err := Register(reg); err != nil {
		t.Fatal(err)
	}
	greg := gates.DefaultRegistry()
	facts, err := reg.Detect(os.DirFS(dir), greg.Has)
	if err != nil {
		t.Fatal(err)
	}
	var checks []plan.Check
	for _, c := range greg.Plan(facts, gates.Options{}) {
		argv := make([]string, len(c.Def.Argv))
		for i, a := range c.Def.Argv {
			argv[i] = strings.ReplaceAll(a, "{target}", c.Target)
		}
		checks = append(checks, plan.Check{ID: c.Def.ID, Kind: c.Def.Kind, Tool: c.Def.Tool, Workdir: c.Workdir, Target: c.Target,
			Argv: argv, Required: c.Required, Substantive: c.Def.Substantive, Path: c.Def.Path})
	}
	plan.SetSelector(Selector)
	t.Cleanup(func() { plan.SetSelector(nil) })
	build := func(rev plan.Revision) *plan.Plan {
		p, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1}, Facts: facts, Checks: checks, Revision: rev,
			World: plan.World{Platform: "linux/amd64", Tools: []string{"go", "node", "pnpm"}, CPU: 4, MemMB: 8192}})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	branch := plan.Revision{Branch: "feature", BaseKnown: true, Changed: []string{"leaf/leaf.go"}}
	p1, p2 := build(branch), build(branch)
	if p1.Selection.Mode != "affected" || p1.Selection.Reason != "selection.affected" {
		t.Fatalf("branch selection = %+v", p1.Selection)
	}
	if len(p1.Selection.Digest) != 64 || p1.Selection.Digest != p2.Selection.Digest {
		t.Fatalf("digest not stable: %q vs %q", p1.Selection.Digest, p2.Selection.Digest)
	}
	have := map[string]bool{}
	for _, j := range p1.Jobs {
		have[j.Job.Workdir] = true
	}
	for _, w := range []string{"leaf", "mid", "app"} {
		if !have[w] {
			t.Errorf("the affected plan lacks a job in %s", w)
		}
	}
	for _, w := range []string{"solo", "web/packages/ui"} {
		if have[w] {
			t.Errorf("the affected plan runs a job in %s", w)
		}
	}
	main := plan.Revision{Branch: "main", DefaultBranch: true, BaseKnown: true, Changed: []string{"leaf/leaf.go"}}
	if p := build(main); p.Selection.Mode != "full" {
		t.Fatalf("default branch selection = %+v", p.Selection)
	}
	rel := plan.Revision{Branch: "v1.0.0", Release: true, BaseKnown: true, Changed: []string{"leaf/leaf.go"}}
	if p := build(rel); p.Selection.Mode != "full" {
		t.Fatalf("release selection = %+v", p.Selection)
	}
	if p := build(plan.Revision{Branch: "feature"}); p.Selection.Mode != "full" || p.Selection.Reason != "selection.no_base" {
		t.Fatalf("no-base selection = %+v", p.Selection)
	}
}
