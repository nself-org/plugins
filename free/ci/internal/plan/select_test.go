package plan

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func noJobs(SelectionInput) (model.Selection, []string) {
	return model.Selection{Mode: "affected", Reason: "selection.affected"}, nil
}

// requiredDeep lists the config jobs that must survive on default-branch and
// release plans.
func requiredDeep(cfg model.PipelineConfig) []string {
	var out []string
	for id, j := range cfg.Jobs {
		if effRequired(j) && effDeep(j) {
			out = append(out, id)
		}
	}
	return out
}

func effRequired(j model.Job) bool { return j.Required == nil || *j.Required }
func effDeep(j model.Job) bool     { return j.Path == "" || j.Path == "deep" }

func TestSelectorFullRequired(t *testing.T) {
	defer SetSelector(nil)
	SetSelector(noJobs)
	cfg := loadConfig(t, "support-matrix.config.json")
	for name, rev := range map[string]Revision{
		"default-branch": {Branch: "main", DefaultBranch: true, BaseKnown: true, Changed: []string{"x/y.go"}},
		"release":        {Branch: "v1", Release: true, BaseKnown: true, Changed: []string{"x/y.go"}},
	} {
		in := matrixInput(t, "xcode")
		in.Revision = rev
		p := mustBuild(t, in)
		for _, id := range requiredDeep(cfg) {
			found := false
			for _, j := range p.Jobs {
				if j.Name == id {
					found = true
					if j.Reason != "selection.full_required" {
						t.Errorf("%s: %s reason = %s, want selection.full_required", name, j.ID, j.Reason)
					}
				}
			}
			if !found {
				t.Errorf("%s: required deep job %s was dropped by the selector", name, id)
			}
		}
		if find(p, "lint") != nil || skippedReason(p, "lint") != "selection.affected" {
			t.Errorf("%s: fast job lint should be skipped by the empty selector", name)
		}
	}
	// A feature branch has no retention: the empty selector selects nothing.
	in := matrixInput(t, "xcode")
	in.Revision = Revision{Branch: "feature/x", BaseKnown: true, Changed: []string{"x/y.go"}}
	if p := mustBuild(t, in); len(p.Jobs) != 0 {
		t.Errorf("feature branch kept %d jobs", len(p.Jobs))
	}
	// SetSelector(nil) restores the path-glob default.
	SetSelector(nil)
	in.Revision.Changed = []string{"tools/a.sh"}
	p := mustBuild(t, in)
	if find(p, "lint") == nil || p.Selection.Mode != "affected" {
		t.Errorf("default selector not restored: %+v", p.Selection)
	}
}

func TestNoBase(t *testing.T) {
	defer SetSelector(nil)
	SetSelector(noJobs) // even a selector that selects nothing cannot beat "no base"
	in := matrixInput(t, "xcode")
	in.Revision = Revision{Branch: "feature/x"}
	p := mustBuild(t, in)
	if p.Selection.Mode != "full" || p.Selection.Reason != "selection.no_base" {
		t.Fatalf("selection = %+v", p.Selection)
	}
	if len(p.Jobs) == 0 {
		t.Fatal("full plan has no jobs")
	}
	for _, j := range p.Jobs {
		if j.Reason != "selection.no_base" {
			t.Errorf("%s reason = %s", j.ID, j.Reason)
		}
	}
	SetSelector(nil)
	if q := mustBuild(t, in); q.Selection.Reason != "selection.no_base" {
		t.Errorf("default selector without base: %+v", q.Selection)
	}
}

func TestDefaultSelector(t *testing.T) {
	jobs := []JobRef{{"root", "."}, {"web", "web"}, {"api", "svc/api"}}
	cases := []struct {
		name    string
		in      SelectionInput
		mode    model.SelectionMode
		reason  model.Reason
		want    []string
		unknown bool
	}{
		{"no base", SelectionInput{Jobs: jobs}, "full", "selection.no_base", []string{"api", "root", "web"}, false},
		{"owned", SelectionInput{BaseKnown: true, Jobs: jobs, Changed: []string{"web/src/a.ts"}}, "affected", "selection.affected", []string{"web"}, false},
		{"root file", SelectionInput{BaseKnown: true, Jobs: jobs, Changed: []string{"README.md"}}, "affected", "selection.affected", []string{"root"}, false},
		{"two", SelectionInput{BaseKnown: true, Jobs: jobs, Changed: []string{"web/a", "svc/api/b"}}, "affected", "selection.affected", []string{"api", "web"}, false},
		{"lockfile", SelectionInput{BaseKnown: true, Jobs: jobs, Changed: []string{"web/package-lock.json"}}, "full", "selection.full_required", []string{"api", "root", "web"}, false},
		{"config", SelectionInput{BaseKnown: true, Jobs: jobs, Changed: []string{".nself/ci.yaml"}}, "full", "selection.full_required", []string{"api", "root", "web"}, false},
		{"unknown fact", SelectionInput{BaseKnown: true, Jobs: jobs, Changed: []string{"web/a"},
			Facts: []detect.Fact{{Kind: "manifest", Name: "unknown", Path: "x/package.json", Confidence: detect.Unknown}}}, "full", "selection.full_required", []string{"api", "root", "web"}, false},
		{"unmapped", SelectionInput{BaseKnown: true, Jobs: jobs[1:], Changed: []string{"docs/a.md"}}, "full", "selection.full_required", []string{"api", "web"}, false},
		{"empty diff", SelectionInput{BaseKnown: true, Jobs: jobs}, "full", "selection.full_required", []string{"api", "root", "web"}, false},
	}
	for _, c := range cases {
		sel, got := DefaultSelector(c.in)
		if sel.Mode != c.mode || sel.Reason != c.reason || len(got) != len(c.want) {
			t.Errorf("%s: got %+v %v", c.name, sel, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: jobs %v, want %v", c.name, got, c.want)
			}
		}
	}
}
