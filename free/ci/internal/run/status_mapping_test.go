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
	"encoding/json"
	"strings"
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
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Error || !strings.HasPrefix(p[0].Description, "error: 0/1 checks passed") {
		t.Fatalf("an error status must not read like a failure: %+v", p)
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
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Failure || !strings.HasPrefix(p[0].Description, "fail: ") {
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
		"network":  noRetry(sh(`echo "fatal: unable to access 'https://github.com/org/repo.git/': Connection timed out after 300001 milliseconds"; exit 128`)),
		"provider": noRetry(sh(`echo "go: Get https://proxy.golang.org/x/@v/list: dial tcp 142.250.1.1:443: i/o timeout"; exit 1`)),
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

// A job that crashes while talking to its own server is a code failure, even
// when it prints a connection reset: an infra class is retried and could go green.
func TestResetFromOwnServerStaysFailure(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"smoke": sh(`echo "curl: (56) Recv failure: Connection reset by peer"; exit 56`)}, gitFake{})
	res := r.run(t)
	if o := outcome(res, "smoke"); o.FailureClass != "code" || o.N != 1 || res.Summary.Result != "fail" {
		t.Fatalf("outcome %+v result %s", o, res.Summary.Result)
	}
}

// A failed prepare of a Node gate reports through the sentinel: an unreachable
// registry is infrastructure (error, retried once), a lockfile out of sync is code.
func TestPrepareFailureReportsThroughTheSentinel(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		log, class string
		result     model.Result
	}{
		"registry unreachable": {"npm error code ETIMEDOUT\nnpm error network request to https://registry.npmjs.org/x failed, reason: connect ETIMEDOUT 1.2.3.4:443", "infra", "error"},
		"lockfile out of sync": {"npm error `npm ci` can only install packages when your package.json and package-lock.json are in sync.", "code", "fail"},
	} {
		t.Run(name, func(t *testing.T) {
			job := noRetry(sh("cat <<'EOF'\n" + tc.log + "\nEOF\necho ::nself-ci-prepare-failed::; exit 97"))
			r := newRig(t, map[string]model.Job{"node": job}, gitFake{})
			res := r.run(t)
			if o := outcome(res, "node"); string(o.FailureClass) != tc.class || res.Summary.Result != tc.result {
				t.Fatalf("outcome %+v result %s", o, res.Summary.Result)
			}
			if tc.class == "infra" {
				if c := checkOf(t, res, "node"); c.Result != "error" || !strings.HasPrefix(c.Remediation, "E811:") {
					t.Fatalf("check %+v", c)
				}
			}
		})
	}
}

// An unbound document (dirty tree, or no commit) is never current: the schema
// forbids binding unbound + freshness current.
func TestUnboundEvidenceIsNeverCurrent(t *testing.T) {
	t.Parallel()
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(0, 0)
	jobs := []JobOutcome{{ID: "a", Required: true, Substantive: true, State: StatePassed}}
	for name, tc := range map[string]struct {
		in    evidenceInput
		fresh model.FreshnessState
	}{
		"dirty":     {evidenceInput{Revision: strings.Repeat("a", 40), Dirty: true}, "unknown"},
		"no commit": {evidenceInput{}, "unknown"},
		"clean":     {evidenceInput{Revision: strings.Repeat("a", 40)}, "current"},
	} {
		in := tc.in
		in.Jobs, in.Start, in.End = jobs, start, start
		in.Trigger, in.Pipeline = "local", model.PipelineRef{ID: "default", Name: "default", Kind: "default"}
		in.Selection = model.Selection{Mode: "full", Reason: "selection.no_base"}
		ev, _, err := buildEvidence(in)
		if err != nil {
			t.Fatal(err)
		}
		if ev.Freshness.State != tc.fresh {
			t.Errorf("%s: freshness %q, want %q", name, ev.Freshness.State, tc.fresh)
		}
		doc, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		if in.Revision == "" { // an empty revision fails the schema pattern on its own; the freshness rule is what is pinned here
			continue
		}
		if err := model.ValidateJSON(schemas["ci-evidence.v1.schema.json"], doc); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
