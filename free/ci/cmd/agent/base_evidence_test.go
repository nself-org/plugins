package main

// Purpose: the single-job base evidence of the agent (D-1004): the per-state verdict, counts and the
// agent-specific producer and runner fields that the shared run constructor must keep.
// Inputs: job outcomes of each state. Outputs: pass/fail.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func TestBaseEvidenceVerdictPerOutcome(t *testing.T) {
	spec := execSpec{CoordinatorID: "coord", AttemptID: "att-1", JobID: "unit", Revision: "0123456789abcdef0123456789abcdef01234567"}
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	end := start.Add(1500 * time.Millisecond)
	base := cirun.JobOutcome{ID: "unit", Name: "unit", Check: "unit", Tool: "nself-ci-agent", Required: true, Substantive: true, N: 1, AttemptID: "att-1"}
	cases := []struct {
		name                    string
		state                   string
		class, code             string
		result, check           string
		pass, fail, errs, skips int
	}{
		{"passed", cirun.StatePassed, "", "", "pass", "pass", 1, 0, 0, 0},
		{"code failure", cirun.StateFailed, "code", "", "fail", "fail", 0, 1, 0, 0},
		{"infra failure", cirun.StateFailed, "infra", "", "error", "error", 0, 0, 1, 0},
		{"coded failure", cirun.StateFailed, "code", "E612", "error", "error", 0, 0, 1, 0},
		{"cancelled", cirun.StateCancelled, "cancelled", "", "cancelled", "skip", 0, 0, 0, 1},
	}
	for _, c := range cases {
		o := base
		o.State, o.FailureClass, o.Code = c.state, model.FailureClass(c.class), c.code
		ev := baseEvidence(spec, o, start, end)
		if string(ev.Result) != c.result || len(ev.Checks) != 1 || string(ev.Checks[0].Result) != c.check {
			t.Errorf("%s: result %q check %+v, want %q/%q", c.name, ev.Result, ev.Checks, c.result, c.check)
		}
		n := ev.Counts
		if n.Total != 1 || n.Pass != c.pass || n.Fail != c.fail || n.Error != c.errs || n.Skip != c.skips {
			t.Errorf("%s: counts %+v", c.name, n)
		}
		if ev.Producer.Kind != "self-reported" || ev.Runner.Provider != "agent" || ev.SourceTrust != "untrusted" || ev.Signature.Kind != "coordinator-recorded" {
			t.Errorf("%s: agent fields %+v %+v %q", c.name, ev.Producer, ev.Runner, ev.SourceTrust)
		}
		if ev.Binding.State != "bound" || ev.Revision != spec.Revision || ev.Pipeline.Kind != "adhoc" || ev.Trigger != "api" || ev.Selection.Mode != "full" {
			t.Errorf("%s: binding/pipeline %+v %+v %q", c.name, ev.Binding, ev.Pipeline, ev.Trigger)
		}
		if ev.Timings.DurationMS == nil || *ev.Timings.DurationMS != 1500 {
			t.Errorf("%s: timings %+v", c.name, ev.Timings)
		}
	}
}
