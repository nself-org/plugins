package planview

// Purpose: P7-AUTO-28 (D-1042). `ci plan` maps all its jobs with SchedJobs and a run maps one
// attempt at a time with SchedJob; both must give the scheduler the same job.

import (
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestSchedJobsSingleMapping(t *testing.T) {
	c := Context{PipelineID: "run1", Project: "/proj", Support: []string{"linux/amd64"}, DefaultPlatform: "linux/amd64", Foreground: true}
	for name, pj := range map[string]PlannedJob{
		"plain":   {ID: "unit", Name: "unit", Job: model.Job{Kind: "test", Run: []string{"go", "test"}}},
		"secrets": {ID: "ship", Name: "ship", Layer: 1, Deps: []string{"unit"}, Job: model.Job{Kind: "release", Run: []string{"ship"}, Secrets: []string{"release/TOKEN", "project/KEY"}, Timeout: "5m"}},
		"remote":  {ID: "farm", Name: "farm", Platform: "linux/arm64", Job: model.Job{Kind: "build", Run: []string{"make"}, RemoteOnly: true}},
		"tooled":  {ID: "lint", Name: "lint", Tool: "golangci-lint", Job: model.Job{Kind: "lint", Run: []string{"lint"}, Isolation: "container"}},
	} {
		many, _ := SchedJobs(c, []PlannedJob{pj})
		if len(many) != 1 {
			t.Fatalf("%s: SchedJobs returned %d jobs", name, len(many))
		}
		one := SchedJob(c, pj)
		// SchedJobs derives Support from the jobs when the context has none; the context here names it, so the two agree.
		if !reflect.DeepEqual(one, many[0]) {
			t.Errorf("%s: a run's mapping differs from the plan's:\n run  %+v\n plan %+v", name, one, many[0])
		}
	}
}
