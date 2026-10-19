package main

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// D-1098: the remote agent maps a failed job to a check and run result with the
// same rule as the local run (model.FailedResult, model.RunVerdict): only the
// code under test can `fail`; a timeout, a lost lease, an infrastructure error
// and an executor code are `error`.
func TestAgentMapsFailedOutcomesLikeTheLocalRun(t *testing.T) {
	for _, tc := range []struct {
		name        string
		o           cirun.JobOutcome
		check, run  string
		fail, errCt int
	}{
		{"test failure", cirun.JobOutcome{State: cirun.StateFailed}, "fail", "fail", 1, 0},
		{"timeout", cirun.JobOutcome{State: cirun.StateFailed, FailureClass: "timeout"}, "error", "error", 0, 1},
		{"lost", cirun.JobOutcome{State: cirun.StateFailed, FailureClass: "lost"}, "error", "error", 0, 1},
		{"infra", cirun.JobOutcome{State: cirun.StateFailed, FailureClass: "infra"}, "error", "error", 0, 1},
		{"executor code", cirun.JobOutcome{State: cirun.StateFailed, Code: "E811"}, "error", "error", 0, 1},
		{"pass", cirun.JobOutcome{State: cirun.StatePassed}, "pass", "pass", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			ev := baseEvidence(execSpec{JobID: "unit", AttemptID: "a1", CoordinatorID: "c1", Revision: rev40}, tc.o, now, now)
			if got := string(ev.Checks[0].Result); got != tc.check {
				t.Errorf("check result %q, want %q", got, tc.check)
			}
			if got := string(ev.Result); got != tc.run {
				t.Errorf("run result %q, want %q", got, tc.run)
			}
			if ev.Counts.Fail != tc.fail || ev.Counts.Error != tc.errCt {
				t.Errorf("counts fail=%d error=%d, want %d and %d", ev.Counts.Fail, ev.Counts.Error, tc.fail, tc.errCt)
			}
			if want := model.FailedResult(tc.o.Code, tc.o.FailureClass); tc.o.State == cirun.StateFailed && ev.Checks[0].Result != want {
				t.Errorf("agent %q differs from model.FailedResult %q", ev.Checks[0].Result, want)
			}
		})
	}
}
