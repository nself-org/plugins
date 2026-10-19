package run

// Purpose: tests of the pure helpers of the dispatch hook (P7-SCHED-06): the
// band an attempt is queued in, and how a dispatcher refusal (E694 included)
// becomes the run's error.

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
)

func TestAttemptBand(t *testing.T) {
	for _, c := range []struct{ declared, trigger, want string }{
		{"", "push", "normal"}, {"", "pull_request", "normal"}, {"", "tag", "normal"}, {"", "api", "normal"},
		{"", "local", "interactive"}, {"", "manual", "interactive"}, {"", "", "interactive"},
		{"", "schedule", "normal"},       // background is held back until P7-SCHED-12 can place it
		{"background", "push", "normal"}, // a declared background is held back the same way
		{"interactive", "push", "interactive"}, {"normal", "manual", "normal"}, {"bogus", "push", "normal"},
	} {
		if got := attemptBand(c.declared, c.trigger); got != c.want {
			t.Errorf("attemptBand(%q, %q) = %q, want %q", c.declared, c.trigger, got, c.want)
		}
	}
}

// TestBackpressureReachesTheRun: E694 from the dispatcher reaches the run with its
// code and the expected wait in the message; other errors pass through.
func TestBackpressureReachesTheRun(t *testing.T) {
	bp := &dispatch.BackpressureError{Err: dispatch.Error{Code: "E694", Message: `project "p" has 3 jobs queued; expected wait about 5s (expected_wait_ms=5000, unknown)`}, ExpectedWaitMs: 5000}
	got := mapDispatchError(bp)
	var re *Error
	if !errors.As(got, &re) || re.Code != "E694" || !strings.Contains(re.Message, "expected_wait_ms=5000") {
		t.Fatalf("E694 must reach the run with its wait: %v", got)
	}
	if other := errors.New("plain"); mapDispatchError(other) != other {
		t.Fatal("other errors pass through")
	}
}
