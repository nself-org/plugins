package run

// Purpose: a failed job that printed nothing still explains itself in the evidence.
// Inputs: outcomes with and without a tail. Outputs: the check's excerpt.

import (
	"strings"
	"testing"
	"time"
)

func TestEvidenceExcerptFallsBackToDetail(t *testing.T) {
	start := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		o    JobOutcome
		want string
	}{
		{"empty tail takes the error text", JobOutcome{ID: "a", State: StateFailed, FailureClass: "infra", Detail: "E612: workdir escapes workspace"}, "E612: workdir escapes workspace"},
		{"a tail wins over the error text", JobOutcome{ID: "b", State: StateFailed, FailureClass: "code", ExitCode: 1, Tail: "FAIL: TestX\n", Detail: "ignored"}, "FAIL: TestX"},
		{"a passed job carries no excerpt", JobOutcome{ID: "c", State: StatePassed, Detail: "ignored"}, ""},
	}
	for _, tc := range cases {
		ev, _, err := buildEvidence(evidenceInput{Jobs: []JobOutcome{tc.o}, Start: start, End: start})
		if err != nil {
			t.Fatal(err)
		}
		if got := ev.Checks[0].Excerpt; got != tc.want {
			t.Errorf("%s: excerpt %q, want %q", tc.name, got, tc.want)
		}
		if len(ev.Checks[0].Excerpt) > maxExcerpt || strings.Contains(ev.Checks[0].Excerpt, "\x00") {
			t.Errorf("%s: excerpt out of bounds", tc.name)
		}
	}
}
