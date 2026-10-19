package run

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func relInfo(result model.Result, kind, check string) RunInfo {
	return RunInfo{RunID: "run-1", Result: result, Evidence: model.Evidence{Checks: []model.Check{{Kind: kind, Result: model.CheckResult(check)}}}}
}

// startRelease dispatches an attempt of the given kind through the hook, on a
// session whose run row names /r as the project.
func startRelease(t *testing.T, h *releasePublish, kind model.JobKind) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	_ = st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: "rev", Project: "/r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"})
	sess := &Session{Store: st, RunID: "run-1", Revision: "rev", Home: "/h"}
	ok := func(context.Context, AttemptSpec) (Admitted, error) { return Admitted{AttemptID: "a"}, nil }
	if _, err := h.dispatch(WithSession(ctx, sess), AttemptSpec{Job: &model.Job{Kind: kind}}, ok); err != nil {
		t.Fatal(err)
	}
}

func withPublisher(t *testing.T, fn func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error)) *int {
	t.Helper()
	old, calls := ReleasePublisher, 0
	ReleasePublisher = func(ctx context.Context, r ReleasePublishRequest) (ReleaseOutcome, error) { calls++; return fn(ctx, r) }
	t.Cleanup(func() { ReleasePublisher = old })
	return &calls
}

func TestReleaseHookPublishesOnlyAPassingReleaseRun(t *testing.T) {
	var got ReleasePublishRequest
	calls := withPublisher(t, func(_ context.Context, r ReleasePublishRequest) (ReleaseOutcome, error) {
		got = r
		return ReleaseOutcome{State: "published", Dir: "/d"}, nil
	})
	h := &releasePublish{}
	// the release was never dispatched through this hook (a remote placement it did not see): fail closed
	if err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err == nil || afterRunExit(err) != 1 || *calls != 0 || !strings.Contains(err.Error(), "nself ci artifacts sign run-1") {
		t.Fatalf("an untracked passing release: %v, %d calls", err, *calls)
	}
	for name, ri := range map[string]RunInfo{"failed run": relInfo("fail", "release", "pass"), "failed check": relInfo("pass", "release", "fail"), "no release check": relInfo("pass", "test", "pass")} {
		startRelease(t, h, "release")
		if err := h.AfterRun(context.Background(), ri); err != nil || *calls != 0 {
			t.Fatalf("%s: %v, %d calls", name, err, *calls)
		}
	}
	startRelease(t, h, "test") // another job kind never registers the run, so a passing release in its evidence is untracked
	if err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); afterRunExit(err) != 1 || *calls != 0 {
		t.Fatalf("a non-release attempt: %v, %d calls", err, *calls)
	}
	startRelease(t, h, "release")
	err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass"))
	if *calls != 1 || got.RunID != "run-1" || got.Home != "/h" || got.Root != "/r" || got.Store == nil || got.Revision != "rev" {
		t.Fatalf("request %+v after %d calls", got, *calls)
	}
	if err == nil || afterRunExit(err) != 0 || !strings.Contains(err.Error(), "published") {
		t.Fatalf("a published release is a note, not a failure: %v", err)
	}
	// the request is dropped once read: a second AfterRun finds the release untracked
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); afterRunExit(err) != 1 || *calls != 1 {
		t.Fatalf("second AfterRun: %v, %d calls", err, *calls)
	}
}

func TestReleaseHookFailureSetsExit(t *testing.T) {
	boom := errors.New("E717 key_missing: no key: run `nself ci artifacts sign run-1`")
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) { return ReleaseOutcome{}, boom })
	h := &releasePublish{}
	startRelease(t, h, "release")
	err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass"))
	if afterRunExit(err) != 1 || !errors.Is(err, boom) {
		t.Fatalf("error %v exit %d", err, afterRunExit(err))
	}
	// nothing to publish and an awaiting message are not failures
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) {
		return ReleaseOutcome{}, ErrNothingToPublish
	})
	startRelease(t, h, "release")
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err != nil {
		t.Fatalf("nothing to publish: %v", err)
	}
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) {
		return ReleaseOutcome{State: "awaiting-signature", Message: "sign it on your machine"}, nil
	})
	startRelease(t, h, "release")
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err == nil || afterRunExit(err) != 0 || err.Error() != "sign it on your machine" {
		t.Fatalf("awaiting: %v", err)
	}
	ReleasePublisher = nil
	startRelease(t, h, "release")
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err != nil {
		t.Fatalf("no publisher installed: %v", err)
	}
}

// A failed publication turns a passing run into exit 1 and prints the recovery
// text as a note, while the evidence stays pass.
func TestFailedPublicationMakesTheRunExitOne(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.AfterRuns = []AfterRunHook{afterFn(func(RunInfo) error {
		return &publishFailure{err: errors.New("E717 signer_failed: boom: run `nself ci artifacts sign run-1`\nsecond line")}
	})}
	res := r.run(t)
	if res.Exit != 1 || res.Summary.Result != "pass" {
		t.Fatalf("exit %d result %s", res.Exit, res.Summary.Result)
	}
	if n := strings.Join(res.Summary.Notes, "\n"); !strings.Contains(n, "nself ci artifacts sign run-1") || strings.Contains(n, "second line") {
		t.Fatalf("notes %q", n)
	}
	// a note-only error leaves the exit alone
	r2 := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r2.C.Hooks.AfterRuns = []AfterRunHook{afterFn(func(RunInfo) error { return errors.New("only a note") })}
	if res2 := r2.run(t); res2.Exit != 0 {
		t.Fatalf("exit %d", res2.Exit)
	}
}

type gateHook struct{ fn func(RunInfo) error }

func (g gateHook) AfterRun(_ context.Context, r RunInfo) error { return g.fn(r) }
func (gateHook) BeforeStatus()                                 {}

type cheerfulPublisher struct{}

func (cheerfulPublisher) Publish(_ context.Context, r PublishRequest) (report.Status, error) {
	return report.Status{State: report.Success, Description: "all green " + r.Run.EvidenceDigest[:8]}, nil
}

// A StatusGate runs before the commit status is posted; its failure is the
// status's too, whatever the registered Publisher says, and only a failure
// that asks for an exit status does it.
func TestPublicationFailureIsTheStatusToo(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err   error
		state report.State
		exit  int
	}{
		"failed publication": {&publishFailure{err: errors.New("E717 key_missing: run `nself ci artifacts sign x`")}, report.Failure, 1},
		"note only":          {releaseNote("release published"), report.Success, 0},
		"published":          {nil, report.Success, 0},
	}
	for name, tc := range cases {
		r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
		r.C.Hooks.Publisher = cheerfulPublisher{}
		var postedBefore int
		r.C.Hooks.AfterRuns = []AfterRunHook{gateHook{func(RunInfo) error { postedBefore = len(r.Rep.all()); return tc.err }}}
		res := r.run(t)
		posted := r.Rep.all()
		if postedBefore != 0 || len(posted) != 1 || posted[0].State != tc.state || res.Exit != tc.exit || res.Summary.Result != "pass" {
			t.Fatalf("%s: posted before the gate %d, statuses %+v, exit %d, result %s", name, postedBefore, posted, res.Exit, res.Summary.Result)
		}
		if tc.state == report.Failure && !strings.Contains(posted[0].Description, "release not published") {
			t.Fatalf("%s: description %q", name, posted[0].Description)
		}
	}
}

// A panic in the publisher is a failed publication (exit 1), and the publisher
// gets a context that outlives the run's spent cleanup deadline.
func TestReleaseHookPanicAndContext(t *testing.T) {
	h := &releasePublish{}
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) { panic("boom") })
	startRelease(t, h, "release")
	err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass"))
	if afterRunExit(err) != 1 || !strings.Contains(err.Error(), "nself ci artifacts sign run-1") || strings.Contains(err.Error(), "boom") {
		t.Fatalf("panic: %v", err)
	}
	var left time.Duration
	withPublisher(t, func(ctx context.Context, _ ReleasePublishRequest) (ReleaseOutcome, error) {
		if d, ok := ctx.Deadline(); ok {
			left = time.Until(d)
		}
		return ReleaseOutcome{State: "published"}, ctx.Err()
	})
	startRelease(t, h, "release")
	spent, cancel := context.WithTimeout(context.Background(), time.Nanosecond) // the run's 15 s context, long gone
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	if err = h.AfterRun(spent, relInfo("pass", "release", "pass")); afterRunExit(err) != 0 || left < time.Minute {
		t.Fatalf("a spent run context reached the publisher: %v, %s left", err, left)
	}
}
