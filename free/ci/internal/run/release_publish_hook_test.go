package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func relInfo(result model.Result, kind, check string) RunInfo {
	return RunInfo{RunID: "run-1", Result: result, Evidence: model.Evidence{Checks: []model.Check{{Kind: kind, Result: model.CheckResult(check)}}}}
}

func startRelease(h *releasePublish, kind model.JobKind) {
	_ = h.AttemptStart(context.Background(), AttemptInfo{RunID: "run-1", Revision: "rev", Home: "/h", Store: &store.Store{}, Job: model.Job{Kind: kind}, SecretsInfo: SecretsInfo{Root: "/r"}})
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
	// no release attempt started: nothing to publish, however the evidence reads
	if err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err != nil || *calls != 0 {
		t.Fatalf("a run without a release attempt: %v, %d calls", err, *calls)
	}
	for name, ri := range map[string]RunInfo{"failed run": relInfo("fail", "release", "pass"), "failed check": relInfo("pass", "release", "fail"), "no release check": relInfo("pass", "test", "pass")} {
		startRelease(h, "release")
		if err := h.AfterRun(context.Background(), ri); err != nil || *calls != 0 {
			t.Fatalf("%s: %v, %d calls", name, err, *calls)
		}
	}
	startRelease(h, "test") // another job kind never registers the run
	if err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err != nil || *calls != 0 {
		t.Fatalf("a non-release attempt: %v, %d calls", err, *calls)
	}
	startRelease(h, "release")
	err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass"))
	if *calls != 1 || got.RunID != "run-1" || got.Home != "/h" || got.Root != "/r" || got.Store == nil {
		t.Fatalf("request %+v after %d calls", got, *calls)
	}
	if err == nil || afterRunExit(err) != 0 || !strings.Contains(err.Error(), "published") {
		t.Fatalf("a published release is a note, not a failure: %v", err)
	}
	// the request is dropped once read
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err != nil || *calls != 1 {
		t.Fatalf("second AfterRun: %v, %d calls", err, *calls)
	}
}

func TestReleaseHookFailureSetsExit(t *testing.T) {
	boom := errors.New("E717 key_missing: no key: run `nself ci artifacts sign run-1`")
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) { return ReleaseOutcome{}, boom })
	h := &releasePublish{}
	startRelease(h, "release")
	err := h.AfterRun(context.Background(), relInfo("pass", "release", "pass"))
	if afterRunExit(err) != 1 || !errors.Is(err, boom) {
		t.Fatalf("error %v exit %d", err, afterRunExit(err))
	}
	// nothing to publish and an awaiting message are not failures
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) {
		return ReleaseOutcome{}, ErrNothingToPublish
	})
	startRelease(h, "release")
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err != nil {
		t.Fatalf("nothing to publish: %v", err)
	}
	withPublisher(t, func(context.Context, ReleasePublishRequest) (ReleaseOutcome, error) {
		return ReleaseOutcome{State: "awaiting-signature", Message: "sign it on your machine"}, nil
	})
	startRelease(h, "release")
	if err = h.AfterRun(context.Background(), relInfo("pass", "release", "pass")); err == nil || afterRunExit(err) != 0 || err.Error() != "sign it on your machine" {
		t.Fatalf("awaiting: %v", err)
	}
	ReleasePublisher = nil
	startRelease(h, "release")
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
