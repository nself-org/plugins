package run

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestPassingRunPostsOneStatus(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("true"), "c": sh("true", "a", "b")}, gitFake{})
	res := r.run(t)
	if res.Exit != 0 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s\n%s", res.Exit, res.Summary.Result, r.Out)
	}
	posted := r.Rep.all()
	if len(posted) != 1 || posted[0].State != report.Success || posted[0].TargetURL != "" || posted[0].Revision != testSHA {
		t.Fatalf("posted %+v", posted)
	}
	if !strings.Contains(posted[0].Description, "evidence "+res.Summary.Evidence[:12]) {
		t.Fatalf("description lacks the evidence digest: %q", posted[0].Description)
	}
}

func TestFailedRequiredJobFailsRunAndSkipsDependants(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("echo boom; exit 3"), "b": sh("true", "a")}, gitFake{})
	res := r.run(t)
	if res.Exit != 1 {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
	if o := outcome(res, "a"); o.State != StateFailed || o.ExitCode != 3 {
		t.Fatalf("a: %+v", o)
	}
	if o := outcome(res, "b"); o.State != StateSkipped || o.Reason != "dep.failed" {
		t.Fatalf("b: %+v", o)
	}
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Failure {
		t.Fatalf("posted %+v", p)
	}
	if !strings.Contains(r.Out.String(), "dep.failed") {
		t.Fatalf("no reason line for the skipped job:\n%s", r.Out)
	}
}

func TestIndependentJobsRunConcurrently(t *testing.T) {
	// each job waits for the other's marker: only concurrent execution passes
	script := func(mine, other string) string {
		return "touch " + mine + "; i=0; while [ ! -e " + other + " ] && [ $i -lt 100 ]; do sleep 0.05; i=$((i+1)); done; test -e " + other
	}
	dir := t.TempDir()
	r := newRig(t, map[string]model.Job{"a": sh(script(dir+"/a", dir+"/b")), "b": sh(script(dir+"/b", dir+"/a"))}, gitFake{})
	if res := r.run(t); res.Exit != 0 {
		t.Fatalf("jobs did not overlap:\n%s", r.Out)
	}
}

func TestFailFastCancelsRunningSiblings(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("sleep 0.3; exit 1"), "b": sh("sleep 30")}, gitFake{})
	r.C.Plan.FailFast = "pipeline"
	start := time.Now()
	res := r.run(t)
	if time.Since(start) > 15*time.Second {
		t.Fatal("the sibling was not cancelled")
	}
	if o := outcome(res, "b"); o.State != StateCancelled || o.Reason != "fail_fast.cancelled" {
		t.Fatalf("b: %+v", o)
	}
	if res.Exit != 1 {
		t.Fatalf("exit %d", res.Exit)
	}
}

func TestStatusReasonLines(t *testing.T) {
	for name, c := range map[string]struct {
		g       gitFake
		noFlag  bool
		want    string
		posts   int
		prepare func(*rig)
	}{
		"no remote":  {g: gitFake{noRemote: true}, want: "nself-ci: status not posted: no GitHub remote"},
		"no auth":    {g: gitFake{noAuth: true}, want: "nself-ci: status not posted: gh not authenticated"},
		"no status":  {noFlag: true, want: "nself-ci: status not posted: --no-status"},
		"dirty tree": {g: gitFake{dirty: true}, want: "nself-ci: status not posted: worktree is dirty"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, map[string]model.Job{"a": sh("true")}, c.g)
			r.C.Opts.NoStatus = c.noFlag
			res := r.run(t)
			if res.Exit != 0 || !strings.Contains(res.Summary.Status, c.want) || !strings.Contains(r.Out.String(), c.want) {
				t.Fatalf("status %q\n%s", res.Summary.Status, r.Out)
			}
			if len(r.Rep.all()) != 0 {
				t.Fatalf("posted despite: %+v", r.Rep.all())
			}
		})
	}
}

func TestUnplaceableRequiredJobFailsAtOnce(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Plan.Jobs[0].State, r.C.Plan.Jobs[0].Blocked, r.C.Plan.Jobs[0].Detail = "unplaceable", "unplaceable.tool", "no such tool"
	_, err := r.C.Run(context.Background())
	if CodeOf(err) != "E618" {
		t.Fatalf("want E618, got %v", err)
	}
	if page, _ := r.store(t).History(context.Background(), 0, 10); len(page.Pipelines) != 0 {
		t.Fatal("a run was persisted before the refusal")
	}
}

func TestJobSelectionRunsDependenciesOnly(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("true", "a"), "c": sh("true")}, gitFake{})
	r.C.Opts.Job = "b"
	res := r.run(t)
	if o := outcome(res, "c"); o.State != StateSkipped || o.Reason != "pipeline.excluded" {
		t.Fatalf("c: %+v", o)
	}
	if outcome(res, "a").State != StatePassed || outcome(res, "b").State != StatePassed {
		t.Fatalf("a/b: %+v", res.Summary.Jobs)
	}
	r2 := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r2.C.Opts.Job = "nope"
	if _, err := r2.C.Run(context.Background()); CodeOf(err) != CodeOptions {
		t.Fatalf("unknown --job: %v", err)
	}
}

func TestCancelFromSecondHandleStopsRun(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("sleep 60"), "b": sh("sleep 60"), "c": sh("true", "a")}, gitFake{})
	done := make(chan Result, 1)
	go func() { res, _ := r.C.Run(context.Background()); done <- res }()
	var st *store.Store
	deadline := time.Now().Add(30 * time.Second)
	var runID string
	for time.Now().Before(deadline) && runID == "" {
		time.Sleep(50 * time.Millisecond)
		if st == nil {
			st = r.store(t)
		}
		if page, err := st.History(context.Background(), 0, 5); err == nil && len(page.Pipelines) == 1 {
			if at, _ := st.PipelineAttempts(context.Background(), page.Pipelines[0].ID); len(at) == 2 && at[0].State == "running" && at[1].State == "running" {
				runID = page.Pipelines[0].ID
			}
		}
	}
	if runID == "" {
		t.Fatal("jobs never reached running")
	}
	t0 := time.Now()
	n, err := CancelRun(context.Background(), st, runID[:8], "test")
	if err != nil || n != 2 {
		t.Fatalf("cancel: n=%d err=%v", n, err)
	}
	select {
	case res := <-done:
		if time.Since(t0) > 15*time.Second {
			t.Fatalf("cancel took %s", time.Since(t0))
		}
		if res.Exit == 0 || outcome(res, "a").State != StateCancelled || outcome(res, "b").State != StateCancelled {
			t.Fatalf("exit %d jobs %+v", res.Exit, res.Summary.Jobs)
		}
		if o := outcome(res, "c"); o.State == StatePassed {
			t.Fatalf("dependant ran: %+v", o)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("run did not stop")
	}
	if _, err = CancelRun(context.Background(), st, "zzz-nope", "x"); CodeOf(err) != CodeRun {
		t.Fatalf("unknown run: %v", err)
	}
}

func TestHooksSeeTheRun(t *testing.T) {
	var order rec
	r := newRig(t, map[string]model.Job{
		"a": sh(`test "$FROM_CACHE" = yes && test "$NSELF_CI_RUN_ID" != ""`),
		"once": func() model.Job {
			j := sh(`test -n "$NSELF_CI_IDEMPOTENCY_KEY"`)
			j.Idempotent = no()
			return j
		}(),
	}, gitFake{})
	cache := &fakeCache{}
	r.C.Hooks = Hooks{Cache: cache, AttemptStarts: []AttemptStartHook{startFn(func(a AttemptInfo) error { order.add("start:" + a.JobName); return nil })},
		AfterRuns:  []AfterRunHook{afterFn(func(ri RunInfo) error { order.add("after:" + string(ri.Result) + ":" + ri.Evidence.Schema); return nil })},
		Finalizers: []Finalizer{fin{"zz", &order}, fin{"aa", &order}}}
	res := r.run(t)
	if res.Exit != 0 {
		t.Fatalf("exit %d\n%s", res.Exit, r.Out)
	}
	if cache.after.Load() != 2 {
		t.Fatalf("cache After ran %d times", cache.after.Load())
	}
	got := strings.Join(order.list(), ",")
	if !strings.Contains(got, "start:a") || !strings.Contains(got, "start:once") || !strings.HasSuffix(got, "after:pass:ci.evidence/v1,fin:aa,fin:zz") {
		t.Fatalf("hook order %s", got)
	}
}

func TestAttemptStartCanFailTheJob(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.AttemptStarts = []AttemptStartHook{startFn(func(AttemptInfo) error { return errors.New("E627: artifact declaration refused") })}
	res := r.run(t)
	if o := outcome(res, "a"); o.State != StateFailed || o.Code != "E627" || res.Exit == 0 {
		t.Fatalf("a: %+v exit %d", o, res.Exit)
	}
}

type liarPublisher struct{}

func (liarPublisher) Publish(_ context.Context, r PublishRequest) (report.Status, error) {
	return report.Status{State: report.Success, Description: "all good " + r.Run.EvidenceDigest[:8]}, nil
}

// A registered Publisher cannot turn a failed run into a success status.
func TestPublisherCannotPostSuccessForFailure(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("exit 1")}, gitFake{})
	r.C.Hooks.Publisher = liarPublisher{}
	if res := r.run(t); res.Exit != 1 {
		t.Fatalf("exit %d", res.Exit)
	}
	if p := r.Rep.all(); len(p) != 1 || p[0].State != report.Failure {
		t.Fatalf("posted %+v", p)
	}
}

// The trust class the classifier returned reaches the stored evidence.
func TestTrustClassInEvidence(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Opts.Trust, r.C.Hooks.Trust = "collaborator", fakeTrust("collaborator")
	res := r.run(t)
	row, ok, err := r.store(t).PipelineByID(context.Background(), res.RunID)
	if err != nil || !ok || row.SourceTrust != "collaborator" {
		t.Fatalf("pipeline source trust %+v %v %v", row, ok, err)
	}
	if res.Summary.Result != "pass" {
		t.Fatal("run failed")
	}
}
