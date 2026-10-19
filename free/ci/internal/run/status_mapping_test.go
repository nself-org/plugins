package run

// Purpose: P7-AUTO-16 pins the status boundary: a run whose required checks
// failed only for infrastructure reasons (a full disk, an unreachable
// registry, a dead runner, a timeout) ends `error` and posts the `error`
// state, never `failure`; a failure of the code under test still ends `fail`
// and posts `failure`.
// Constraints: real shell jobs through the coordinator rig; the `lost` leg is
// a unit test of the evidence mapping because a lost lease needs a second
// coordinator.

import (
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// noRetry stops the infra retry so each case ends after one attempt.
func noRetry(j model.Job) model.Job { j.Retry = &model.RetryConfig{InfraMax: 0}; return j }

func TestInfraPostsError(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"unit": noRetry(sh(`echo "tar: write error: No space left on device"; exit 1`))}, gitFake{})
	res := r.run(t)
	if o := outcome(res, "unit"); o.FailureClass != "infra" {
		t.Fatalf("outcome %+v\n%s", o, r.Out)
	}
	if res.Summary.Result != "error" || res.Exit != 2 {
		t.Fatalf("an infrastructure failure must end error (exit 2), got %s exit %d", res.Summary.Result, res.Exit)
	}
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Error {
		t.Fatalf("posted %+v", p)
	}
}

func TestCodeFailurePostsFailure(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"unit": noRetry(sh(`echo "FAIL: TestThing"; exit 1`))}, gitFake{})
	res := r.run(t)
	if o := outcome(res, "unit"); o.FailureClass != "code" {
		t.Fatalf("outcome %+v", o)
	}
	if res.Summary.Result != "fail" || res.Exit != 1 {
		t.Fatalf("a code failure must end fail (exit 1), got %s exit %d", res.Summary.Result, res.Exit)
	}
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Failure {
		t.Fatalf("posted %+v", p)
	}
}

// A real failing job beside an infrastructure one is still a failure: the
// code is wrong whatever the runner did.
func TestCodeFailureBesideInfraStaysFailure(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{
		"disk": noRetry(sh(`echo "no space left on device"; exit 1`)),
		"unit": noRetry(sh(`echo "FAIL: TestThing"; exit 1`)),
	}, gitFake{})
	res := r.run(t)
	if res.Summary.Result != "fail" {
		t.Fatalf("got %s\n%s", res.Summary.Result, r.Out)
	}
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Failure {
		t.Fatalf("posted %+v", p)
	}
}

func TestTimeoutLostDiskNetworkEndError(t *testing.T) {
	t.Parallel()
	timeout := noRetry(sh(`sleep 30`))
	timeout.Timeout = "1s"
	for name, job := range map[string]model.Job{
		"timeout":  timeout,
		"disk":     noRetry(sh(`echo "ENOSPC: no space left on device, write"; exit 1`)),
		"network":  noRetry(sh(`echo "npm ERR! code ETIMEDOUT"; exit 1`)),
		"provider": noRetry(sh(`echo "curl: (56) Recv failure: Connection reset by peer"; exit 1`)),
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, map[string]model.Job{"unit": job}, gitFake{})
			res := r.run(t)
			if c := checkOf(t, res, "unit"); c.Result != "error" {
				t.Fatalf("check %+v\n%s", c, r.Out)
			}
			if res.Summary.Result != "error" {
				t.Fatalf("run ended %s", res.Summary.Result)
			}
			if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Error {
				t.Fatalf("posted %+v", p)
			}
		})
	}
}

// The mapping itself, for the classes a rig cannot produce cheaply.
func TestCheckResultMapping(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		o    JobOutcome
		want model.CheckResult
	}{
		"passed":        {JobOutcome{State: StatePassed}, "pass"},
		"code":          {JobOutcome{State: StateFailed, FailureClass: "code"}, "fail"},
		"infra":         {JobOutcome{State: StateFailed, FailureClass: "infra"}, "error"},
		"timeout":       {JobOutcome{State: StateFailed, FailureClass: "timeout"}, "error"},
		"lost":          {JobOutcome{State: StateFailed, FailureClass: "lost"}, "error"},
		"coded refusal": {JobOutcome{State: StateFailed, Code: "E612"}, "error"},
		"skipped":       {JobOutcome{State: StateSkipped}, "skip"},
	} {
		if got := checkResult(tc.o); got != tc.want {
			t.Errorf("%s: %s, want %s", name, got, tc.want)
		}
	}
}

func TestRunResultFromChecks(t *testing.T) {
	t.Parallel()
	start := time.Unix(0, 0)
	for name, tc := range map[string]struct {
		jobs []JobOutcome
		want model.Result
	}{
		"lost runner":    {[]JobOutcome{{ID: "a", Required: true, Substantive: true, State: StateFailed, FailureClass: "lost"}}, "error"},
		"infra + pass":   {[]JobOutcome{{ID: "a", Required: true, Substantive: true, State: StateFailed, FailureClass: "infra"}, {ID: "b", Required: true, Substantive: true, State: StatePassed}}, "error"},
		"code + infra":   {[]JobOutcome{{ID: "a", Required: true, Substantive: true, State: StateFailed, FailureClass: "infra"}, {ID: "b", Required: true, Substantive: true, State: StateFailed, FailureClass: "code"}}, "fail"},
		"advisory infra": {[]JobOutcome{{ID: "a", Required: false, Substantive: true, State: StateFailed, FailureClass: "infra"}, {ID: "b", Required: true, Substantive: true, State: StatePassed}}, "pass"},
		"nothing ran":    {nil, "fail"},
	} {
		ev, _, err := buildEvidence(evidenceInput{Jobs: tc.jobs, Start: start, End: start})
		if err != nil {
			t.Fatal(err)
		}
		if ev.Result != tc.want {
			t.Errorf("%s: %s, want %s", name, ev.Result, tc.want)
		}
	}
}
