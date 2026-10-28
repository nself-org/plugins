package run

// Purpose: SingleJobEvidence is buildEvidence for one job plus the caller's identity fields (D-1004).

import (
	"reflect"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestSingleJobEvidenceMatchesBuildEvidence(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(time.Second)
	job := JobOutcome{ID: "unit", Check: "unit", Tool: "t", Required: true, Substantive: true, State: StateFailed, FailureClass: "infra", Tail: "boom"}
	in := SingleJobInput{RunID: "r1", Revision: "abc", RepoID: "repo", Source: "untrusted", Pipeline: model.PipelineRef{ID: "r1", Name: "unit", Kind: "adhoc"},
		Trigger: "api", Selection: model.Selection{Mode: "full", Reason: "selection.full_required"},
		Runner: model.RunnerRef{ID: "agent", Provider: "agent", Isolation: "process"}, Producer: model.Producer{Kind: "self-reported", Version: "1"},
		Job: job, Start: start, End: end}
	got := SingleJobEvidence(in)
	want, _, err := buildEvidence(evidenceInput{RunID: "r1", Revision: "abc", Source: "untrusted", Pipeline: in.Pipeline, Selection: in.Selection, Trigger: "api",
		Jobs: []JobOutcome{job}, Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	want.RepoID, want.Runner, want.Producer = in.RepoID, in.Runner, in.Producer
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got.Result != "error" || got.Checks[0].Result != "error" || got.Counts.Error != 1 {
		t.Fatalf("an infra failure is an error verdict, got %q %+v", got.Result, got.Counts)
	}
	job.State = StateCancelled
	if c := SingleJobEvidence(SingleJobInput{Job: job, Start: start, End: end}); c.Result != "cancelled" {
		t.Fatalf("cancelled job: result %q", c.Result)
	}
}
