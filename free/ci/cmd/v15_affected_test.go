package main

import (
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/affected"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

const affectedFixture = "../internal/affected/testdata/monorepo"

func countPkggraph(t *testing.T) int {
	t.Helper()
	facts, _, err := planChecks(affectedFixture, model.PipelineConfig{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range facts {
		if f.Kind == affected.FactKind {
			n++
		}
	}
	return n
}

func TestPlanDetectorsOnlyInV15(t *testing.T) {
	t.Setenv("NSELF_V15", "0")
	if n := countPkggraph(t); n != 0 {
		t.Fatalf("v1.4 produced %d pkggraph facts", n)
	}
	t.Setenv("NSELF_V15", "1")
	if n := countPkggraph(t); n == 0 {
		t.Fatal("v1.5 produced no pkggraph facts")
	}
}

func TestAffectedSelectorFollowsMode(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	facts, _, err := planChecks(affectedFixture, model.PipelineConfig{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	in := plan.SelectionInput{Changed: []string{"leaf/leaf.go"}, BaseKnown: true, Facts: facts, Jobs: []plan.JobRef{
		{ID: "leaf", Workdir: "leaf"}, {ID: "mid", Workdir: "mid"}, {ID: "app", Workdir: "app"}, {ID: "solo", Workdir: "solo"}}}
	sel, jobs := affectedSelector(in)
	if sel.Mode != "affected" || !reflect.DeepEqual(jobs, []string{"app", "leaf", "mid"}) {
		t.Fatalf("v1.5 selection = %+v %v", sel, jobs)
	}
	t.Setenv("NSELF_V15", "0")
	want, wantJobs := plan.DefaultSelector(in)
	got, gotJobs := affectedSelector(in)
	if got.Mode != want.Mode || !reflect.DeepEqual(gotJobs, wantJobs) {
		t.Fatalf("v1.4 selection = %+v %v, want the default %+v %v", got, gotJobs, want, wantJobs)
	}
}
