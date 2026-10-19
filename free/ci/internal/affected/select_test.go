package affected

import (
	"os"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// fixtureJobs are the jobs a plan of the monorepo fixture holds: one per Go
// module and one per pnpm package, by working directory.
func fixtureJobs() []plan.JobRef {
	return []plan.JobRef{
		{ID: "app.test", Workdir: "app"}, {ID: "leaf.test", Workdir: "leaf"}, {ID: "mid.test", Workdir: "mid"},
		{ID: "solo.test", Workdir: "solo"}, {ID: "lonely.test", Workdir: "web/packages/lonely"},
		{ID: "site.test", Workdir: "web/packages/site"}, {ID: "ui.test", Workdir: "web/packages/ui"},
	}
}

func fixtureFacts(t *testing.T) []detect.Fact {
	t.Helper()
	g, err := Build(os.DirFS("testdata/monorepo"))
	if err != nil {
		t.Fatal(err)
	}
	return g.Facts()
}

func input(t *testing.T, changed ...string) plan.SelectionInput {
	return plan.SelectionInput{Changed: changed, BaseKnown: true, Jobs: fixtureJobs(), Facts: fixtureFacts(t)}
}

func TestSelectorCases(t *testing.T) {
	all := []string{"app.test", "leaf.test", "lonely.test", "mid.test", "site.test", "solo.test", "ui.test"}
	cases := []struct {
		name    string
		mutate  func(*plan.SelectionInput)
		changed []string
		mode    string
		reason  model.Reason
		jobs    []string
	}{
		{"leaf selects its dependants transitively", nil, []string{"leaf/leaf.go"}, "affected", "selection.affected", []string{"app.test", "leaf.test", "mid.test"}},
		{"mid selects mid and app", nil, []string{"mid/mid.go"}, "affected", "selection.affected", []string{"app.test", "mid.test"}},
		{"app selects only app", nil, []string{"app/main.go"}, "affected", "selection.affected", []string{"app.test"}},
		{"solo is independent", nil, []string{"solo/solo.go"}, "affected", "selection.affected", []string{"solo.test"}},
		{"pnpm ui selects site", nil, []string{"web/packages/ui/src/x.ts"}, "affected", "selection.affected", []string{"site.test", "ui.test"}},
		{"excluded pnpm package stays alone", nil, []string{"web/packages/lonely/package.json"}, "affected", "selection.affected", []string{"lonely.test"}},
		{"two packages union", nil, []string{"solo/solo.go", "app/main.go"}, "affected", "selection.affected", []string{"app.test", "solo.test"}},
		{"a file in a package subtree maps to the package", nil, []string{"mid/testdata/golden.json"}, "affected", "selection.affected", []string{"app.test", "mid.test"}},
		{"go.sum", nil, []string{"leaf/go.sum"}, "full", "selection.full_required", all},
		{"go.mod", nil, []string{"mid/go.mod"}, "full", "selection.full_required", all},
		{"pnpm lock", nil, []string{"pnpm-lock.yaml"}, "full", "selection.full_required", all},
		{"ci config", nil, []string{".nself/ci.yaml"}, "full", "selection.full_required", all},
		{"build script", nil, []string{"scripts/build.sh"}, "full", "selection.full_required", all},
		{"makefile in a package", nil, []string{"mid/Makefile"}, "full", "selection.full_required", all},
		{"dockerfile", nil, []string{"app/Dockerfile"}, "full", "selection.full_required", all},
		{"generator input", nil, []string{"leaf/api.proto"}, "full", "selection.full_required", all},
		{"root manifest", nil, []string{"package.json"}, "full", "selection.full_required", all},
		{"workflow", nil, []string{".github/workflows/ci.yml"}, "full", "selection.full_required", all},
		{"unmapped path", nil, []string{"docs/guide.md"}, "full", "selection.full_required", all},
		{"outside the repository", nil, []string{"../x"}, "full", "selection.full_required", all},
		{"empty diff", nil, nil, "full", "selection.full_required", all},
		{"no base", func(in *plan.SelectionInput) { in.BaseKnown = false }, []string{"leaf/leaf.go"}, "full", "selection.no_base", all},
		{"default branch", func(in *plan.SelectionInput) { in.DefaultBranch = true }, []string{"leaf/leaf.go"}, "full", "selection.full_required", all},
		{"release", func(in *plan.SelectionInput) { in.Release = true }, []string{"leaf/leaf.go"}, "full", "selection.full_required", all},
		{"release pipeline", func(in *plan.SelectionInput) { in.ReleasePipeline = true }, []string{"leaf/leaf.go"}, "full", "selection.full_required", all},
		{"uncertain fact elsewhere", func(in *plan.SelectionInput) {
			in.Facts = append(in.Facts, detect.Fact{Kind: "manifest", Name: "unknown", Path: "x", Confidence: detect.Unknown})
		}, []string{"leaf/leaf.go"}, "full", "selection.full_required", all},
		{"uncertain graph", func(in *plan.SelectionInput) {
			in.Facts = append(in.Facts, detect.Fact{Kind: FactKind, Name: "unknown", Path: "web", Confidence: detect.Unknown, Reason: "x"})
		}, []string{"leaf/leaf.go"}, "full", "selection.full_required", all},
		{"covering job at the repository root", func(in *plan.SelectionInput) {
			in.Jobs = append(in.Jobs, plan.JobRef{ID: "root.lint", Workdir: "."})
		}, []string{"solo/solo.go"}, "affected", "selection.affected", []string{"root.lint", "solo.test"}},
		{"a job under an affected package", func(in *plan.SelectionInput) {
			in.Jobs = append(in.Jobs, plan.JobRef{ID: "e2e", Workdir: "mid/e2e"})
		}, []string{"mid/mid.go"}, "affected", "selection.affected", []string{"app.test", "e2e", "mid.test"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := input(t, c.changed...)
			if c.mutate != nil {
				c.mutate(&in)
			}
			sel, jobs := Selector(in)
			if string(sel.Mode) != c.mode || sel.Reason != c.reason {
				t.Fatalf("selection = %s/%s, want %s/%s", sel.Mode, sel.Reason, c.mode, c.reason)
			}
			if !reflect.DeepEqual(jobs, c.jobs) {
				t.Fatalf("jobs = %v, want %v", jobs, c.jobs)
			}
			if len(sel.Digest) != 64 || sel.Digest != Digest(normalize(c.changed), jobs) {
				t.Fatalf("digest = %q", sel.Digest)
			}
		})
	}
}

func TestSelectorWithoutGraphFacts(t *testing.T) {
	jobs := []plan.JobRef{{ID: "root", Workdir: "."}, {ID: "tools", Workdir: "tools"}}
	in := plan.SelectionInput{BaseKnown: true, Jobs: jobs}
	for _, c := range []struct {
		changed []string
		mode    string
		ids     []string
	}{
		{[]string{"tools/x.txt"}, "affected", []string{"root", "tools"}}, // every covering workdir
		{[]string{"config.json"}, "full", []string{"root", "tools"}},     // unowned root file
		{[]string{"Makefile"}, "full", []string{"root", "tools"}},
	} {
		in.Changed = c.changed
		sel, ids := Selector(in)
		if string(sel.Mode) != c.mode || !reflect.DeepEqual(ids, c.ids) {
			t.Errorf("%v: %v %v, want %s %v", c.changed, sel, ids, c.mode, c.ids)
		}
	}
}

func TestDigestIsStableAndSensitive(t *testing.T) {
	a := Digest([]string{"b", "a"}, []string{"y", "x"})
	if a != Digest([]string{"a", "b"}, []string{"x", "y"}) {
		t.Fatal("digest depends on input order")
	}
	if a == Digest([]string{"a", "b"}, []string{"x"}) || a == Digest([]string{"a"}, []string{"x", "y"}) {
		t.Fatal("digest ignores a changed path or a job")
	}
}
