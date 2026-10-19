package run

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// D-1004: the agent builds its one-job document through the exported
// SingleJobEvidence. It must agree with the coordinator's buildEvidence on the
// checks, counts and verdict, and differ only in the identity it claims.
func TestSingleJobEvidenceAgreesWithBuildEvidence(t *testing.T) {
	start := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Second)
	base := JobOutcome{ID: "unit", Name: "unit", Check: "unit", Tool: "nself-ci-agent", Kind: "test", Required: true, Substantive: true, N: 1}
	cases := map[string]func(*JobOutcome){
		"passed":    func(o *JobOutcome) { o.State = StatePassed },
		"failed":    func(o *JobOutcome) { o.State, o.FailureClass, o.Tail = StateFailed, "test", "boom" },
		"infra":     func(o *JobOutcome) { o.State, o.FailureClass = StateFailed, "infra" },
		"coded":     func(o *JobOutcome) { o.State, o.Code, o.Tail = StateFailed, "E612", "no secrets" },
		"cancelled": func(o *JobOutcome) { o.State, o.FailureClass = StateCancelled, "cancelled" },
		"long tail": func(o *JobOutcome) { o.State, o.Tail = StateFailed, strings.Repeat("x", maxExcerpt+50) },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			o := base
			mod(&o)
			rev := strings.Repeat("a", 40)
			got, err := SingleJobEvidence(SingleJob{RunID: "r1", Revision: rev, Name: "unit", Outcome: o, Start: start, End: end})
			if err != nil {
				t.Fatal(err)
			}
			want, _, err := buildEvidence(evidenceInput{RunID: "r1", Revision: rev, Jobs: []JobOutcome{o}, Cancelled: o.State == StateCancelled, Start: start, End: end})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Checks, want.Checks) || got.Counts != want.Counts || got.Result != want.Result || !reflect.DeepEqual(got.Timings, want.Timings) {
				t.Fatalf("single-job document drifted from buildEvidence:\n got %+v\nwant %+v", got, want)
			}
			if got.Producer.Kind != "self-reported" || got.Runner.Provider != "agent" || got.SourceTrust != "untrusted" || got.RepoID != "unknown" {
				t.Fatalf("agent identity not claimed: %+v %+v %q %q", got.Producer, got.Runner, got.SourceTrust, got.RepoID)
			}
			if got.Pipeline.Kind != "adhoc" || got.Pipeline.Name != "unit" || got.Revision != rev || got.Binding.State != "bound" {
				t.Fatalf("pipeline or binding wrong: %+v %q %+v", got.Pipeline, got.Revision, got.Binding)
			}
		})
	}
}
