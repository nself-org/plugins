package run

// Purpose: what lets a job's waiters run (D-1119). A failed allow_failure job
// blocks them as plan/dag.go promises; a prerequisite that never ran blocks them too.

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestDepOK(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		d    JobOutcome
		want bool
	}{
		{"passed", JobOutcome{State: StatePassed}, true},
		{"failed", JobOutcome{State: StateFailed}, false},
		{"failed allow_failure", JobOutcome{State: StateFailed, AllowFailure: true}, false},
		{"cancelled", JobOutcome{State: StateCancelled}, false},
		{"skipped not affected", JobOutcome{State: StateSkipped, Reason: "selection.affected"}, true},
		{"skipped by when", JobOutcome{State: StateSkipped, Reason: "when.paths"}, true},
		{"skipped dep.failed", JobOutcome{State: StateSkipped, Reason: "dep.failed"}, false},
		{"skipped fail-fast", JobOutcome{State: StateSkipped, Reason: "fail_fast.cancelled"}, false},
		{"skipped unplaceable", JobOutcome{State: StateSkipped, Reason: "unplaceable.tool"}, false},
		{"skipped admission", JobOutcome{State: StateSkipped, Reason: "admission.battery"}, false},
		{"skipped advisory", JobOutcome{State: StateSkipped, Reason: "advisory.tool_missing"}, false},
	}
	for _, tc := range cases {
		if got := depOK(tc.d); got != tc.want {
			t.Errorf("%s: depOK = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestFailedAllowFailureJobBlocksItsWaiters(t *testing.T) {
	t.Parallel()
	opt := func(script string, deps ...string) model.Job {
		j := sh(script, deps...)
		j.Required = no()
		return j
	}
	build := opt("exit 1")
	build.AllowFailure = true
	r := newRig(t, map[string]model.Job{"build": build, "publish": opt("true", "build")}, gitFake{})
	res := r.run(t)
	if o := outcome(res, "build"); o.State != StateFailed {
		t.Fatalf("build: %+v", o)
	}
	if o := outcome(res, "publish"); o.State != StateSkipped || o.Reason != "dep.failed" {
		t.Fatalf("publish ran or was skipped for the wrong reason: %+v", o)
	}
}
