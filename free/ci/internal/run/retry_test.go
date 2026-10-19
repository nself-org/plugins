package run

// P7-CI-40: failure classes in evidence, bounded infra retries (through
// CreateAttempt and the dispatch hook), and explicit flaky confirmation.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
)

// counting is a script that appends a line to marker before running body, so a
// test can count attempts and branch on which attempt it is (n = lines so far).
func counting(marker, body string) model.Job {
	return sh(`n=$(wc -l < ` + marker + `); echo x >> ` + marker + `; ` + body)
}

func attemptsOf(t *testing.T, r *rig, res Result) []string {
	t.Helper()
	st := r.store(t)
	at, err := st.PipelineAttempts(context.Background(), res.Summary.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range at {
		out = append(out, a.State+"/"+string(a.FailureClass))
	}
	return out
}

func newMarker(t *testing.T) string {
	t.Helper()
	m := filepath.Join(t.TempDir(), "runs")
	if err := os.WriteFile(m, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return m
}

func checkOf(t *testing.T, res Result, id string) model.Check {
	t.Helper()
	for _, c := range res.Summary.Doc.Checks {
		if c.JobID == id {
			return c
		}
	}
	t.Fatalf("no check for job %s in the evidence", id)
	return model.Check{}
}

func TestInfraFailureIsRetriedOnceThenFailsAsInfra(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	r := newRig(t, map[string]model.Job{"mem": counting(m, `echo "Out of memory: Killed process 4242 (cc1)"; exit 137`)}, gitFake{})
	res := r.run(t)
	o := outcome(res, "mem")
	if o.State != StateFailed || o.FailureClass != "infra" || o.N != 2 || len(o.Retries) != 1 {
		t.Fatalf("outcome %+v\n%s", o, r.Out)
	}
	if got := attemptsOf(t, r, res); strings.Join(got, ",") != "cancelled/infra,failed/infra" {
		t.Fatalf("attempts %v, want the superseded first attempt and one failed infra attempt", got)
	}
	if b, _ := os.ReadFile(m); strings.Count(string(b), "x") != 2 {
		t.Fatalf("job ran %d times, want 2", strings.Count(string(b), "x"))
	}
	c := checkOf(t, res, "mem")
	if c.FailureClass != "infra" || c.Diagnosis != "infra.oom" || c.Exit == nil || *c.Exit != 137 || c.Remediation == "" || !strings.Contains(c.Excerpt, "Out of memory") {
		t.Fatalf("check %+v", c)
	}
	if rt := res.Summary.Doc.Retries; len(rt) != 1 || rt[0].JobID != "mem" || rt[0].Attempt != 2 || rt[0].Class != "infra" {
		t.Fatalf("retries %+v", rt)
	}
	if res.Exit == 0 || res.Summary.Result == "pass" {
		t.Fatalf("a failed required job passed: exit %d result %s", res.Exit, res.Summary.Result)
	}
}

func TestInfraRetryThatPassesLeavesTheRetryVisible(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	r := newRig(t, map[string]model.Job{"disk": counting(m, `if [ "$n" -eq 0 ]; then echo "write: no space left on device"; exit 1; fi; exit 0`)}, gitFake{})
	res := r.run(t)
	o := outcome(res, "disk")
	if o.State != StatePassed || o.N != 2 || len(o.Retries) != 1 || o.Retries[0].Class != "infra" {
		t.Fatalf("outcome %+v\n%s", o, r.Out)
	}
	if got := attemptsOf(t, r, res); strings.Join(got, ",") != "cancelled/infra,passed/" {
		t.Fatalf("attempts %v", got)
	}
	if len(res.Summary.Doc.Retries) != 1 {
		t.Fatalf("retries %+v", res.Summary.Doc.Retries)
	}
}

func TestCodeFailureIsNeverRetried(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	job := counting(m, `echo "--- FAIL: TestThing"; exit 1`)
	job.Retry = &model.RetryConfig{InfraMax: 3}
	r := newRig(t, map[string]model.Job{"unit": job}, gitFake{})
	res := r.run(t)
	o := outcome(res, "unit")
	if o.State != StateFailed || o.FailureClass != "code" || o.N != 1 || len(o.Retries) != 0 {
		t.Fatalf("outcome %+v", o)
	}
	if b, _ := os.ReadFile(m); strings.Count(string(b), "x") != 1 {
		t.Fatalf("a code failure ran %d times", strings.Count(string(b), "x"))
	}
	if c := checkOf(t, res, "unit"); c.Diagnosis != "code" || c.Exit == nil || *c.Exit != 1 || c.Remediation == "" || !strings.Contains(c.Excerpt, "FAIL: TestThing") {
		t.Fatalf("check %+v", c)
	}
}

func TestTimeoutIsNotRetried(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	job := counting(m, `sleep 30`)
	job.Timeout = "1s"
	r := newRig(t, map[string]model.Job{"slow": job}, gitFake{})
	res := r.run(t)
	o := outcome(res, "slow")
	if o.State != StateFailed || o.FailureClass != "timeout" || o.N != 1 {
		t.Fatalf("outcome %+v\n%s", o, r.Out)
	}
	if c := checkOf(t, res, "slow"); c.FailureClass != "timeout" || c.Diagnosis != "timeout" || c.Remediation == "" {
		t.Fatalf("check %+v", c)
	}
}

func TestInfraBudgetFollowsRetryConfigAndIdempotence(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mut      func(*model.Job)
		attempts int
	}{
		"infra_max 3":       {func(j *model.Job) { j.Retry = &model.RetryConfig{InfraMax: 3} }, 4},
		"infra_max 0":       {func(j *model.Job) { j.Retry = &model.RetryConfig{InfraMax: 0} }, 1},
		"idempotent false":  {func(j *model.Job) { j.Idempotent = no() }, 1},
		"default infra_max": {func(j *model.Job) {}, 2},
	} {
		m := newMarker(t)
		job := counting(m, `echo "Cannot connect to the Docker daemon"; exit 1`)
		tc.mut(&job)
		r := newRig(t, map[string]model.Job{"j": job}, gitFake{})
		res := r.run(t)
		if o := outcome(res, "j"); o.FailureClass != "infra" || o.N != tc.attempts {
			t.Errorf("%s: outcome %+v, want %d attempts", name, o, tc.attempts)
		}
		if b, _ := os.ReadFile(m); strings.Count(string(b), "x") != tc.attempts {
			t.Errorf("%s: job ran %d times, want %d", name, strings.Count(string(b), "x"), tc.attempts)
		}
	}
}

func TestInfraRetriesReachTheDispatchHook(t *testing.T) {
	t.Cleanup(func() { Dispatch = nil })
	spy := &spyDispatch{}
	Dispatch = spy.hook
	m := newMarker(t)
	r := newRig(t, map[string]model.Job{"net": counting(m, `echo "curl: (6) Could not resolve host: example.test"; exit 6`)}, gitFake{})
	res := r.run(t)
	if len(spy.seen) != 2 {
		t.Fatalf("dispatch saw %d attempts, want 2 (the first and one infra retry): %v", len(spy.seen), spy.seen)
	}
	if o := outcome(res, "net"); o.FailureClass != "infra" || o.N != 2 {
		t.Fatalf("outcome %+v", o)
	}
}

func TestConfirmFlakyRecordsRerunsAndKeepsTheFailure(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	r := newRig(t, map[string]model.Job{"unit": counting(m, `if [ "$n" -lt 2 ]; then echo "--- FAIL: TestRace"; exit 1; fi; exit 0`)}, gitFake{})
	r.C.Opts.ConfirmFlaky = 3
	res := r.run(t)
	o := outcome(res, "unit")
	if o.State != StateFailed || o.FailureClass != "code" {
		t.Fatalf("the original failure changed: %+v", o)
	}
	if o.Flaky == nil || o.Flaky.Source != "confirmation" || o.Flaky.Passes != 1 || o.Flaky.Fails != 2 || len(o.Retries) != 2 {
		t.Fatalf("flaky note %+v retries %+v", o.Flaky, o.Retries)
	}
	if res.Exit == 0 || res.Summary.Result == "pass" {
		t.Fatalf("a flaky check read as a pass: exit %d result %s", res.Exit, res.Summary.Result)
	}
	c := checkOf(t, res, "unit")
	if c.Result != "fail" || c.Flaky == nil || len(res.Summary.Doc.Retries) != 2 {
		t.Fatalf("check %+v retries %+v", c, res.Summary.Doc.Retries)
	}
	if got := attemptsOf(t, r, res); strings.Join(got, ",") != "failed/code,failed/code,passed/" {
		t.Fatalf("attempts %v", got)
	}
}

func TestConfirmFlakyOnADeterministicFailureIsNotAFlake(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	r := newRig(t, map[string]model.Job{"unit": counting(m, `exit 1`)}, gitFake{})
	r.C.Opts.ConfirmFlaky = 2
	o := outcome(r.run(t), "unit")
	if o.Flaky != nil || len(o.Retries) != 2 || o.State != StateFailed {
		t.Fatalf("outcome %+v", o)
	}
}

func TestNoConfirmationWithoutTheFlagOrForOnceOnlyJobs(t *testing.T) {
	t.Parallel()
	m := newMarker(t)
	r := newRig(t, map[string]model.Job{"unit": counting(m, `exit 1`)}, gitFake{})
	if o := outcome(r.run(t), "unit"); len(o.Retries) != 0 || o.N != 1 {
		t.Fatalf("reran a code failure without --confirm-flaky: %+v", o)
	}
	m2 := newMarker(t)
	once := counting(m2, `exit 1`)
	once.Idempotent = no()
	r2 := newRig(t, map[string]model.Job{"once": once}, gitFake{})
	r2.C.Opts.ConfirmFlaky = 3
	if o := outcome(r2.run(t), "once"); len(o.Retries) != 0 || o.N != 1 {
		t.Fatalf("reran an idempotent:false job: %+v", o)
	}
}

func TestNoRetryAfterCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	rs := &runState{marks: map[string]model.Reason{}}
	pj := planned("a")
	if !rs.stillRunning(ctx, pj) {
		t.Fatal("a live run must be allowed another attempt")
	}
	rs.marks["a"] = "fail_fast.cancelled"
	if rs.stillRunning(ctx, pj) {
		t.Fatal("a fail-fast marked job was retried")
	}
	delete(rs.marks, "a")
	rs.userCancel = true
	if rs.stillRunning(ctx, pj) {
		t.Fatal("an operator-cancelled run was retried")
	}
	rs.userCancel = false
	cancel()
	if rs.stillRunning(ctx, pj) {
		t.Fatal("a cancelled context was retried")
	}
}

func planned(id string) plan.PlannedJob { return plan.PlannedJob{ID: id} }
