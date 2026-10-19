package job_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// runFixture is a stored run whose build and lint jobs passed.
type runFixture struct {
	st      *store.Store
	sha     string
	dirty   bool
	bound   model.InputDigests
	now     model.InputDigests
	inErr   error
	asked   int
	gather  job.Gatherer
	attempt job.Attempt
}

func newRun(t *testing.T, statuses map[string]string) *runFixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &runFixture{st: st, sha: revision, bound: digests(), now: digests()}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: revision, Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "policy-1", InputDigest: f.bound.Config, SelectionMode: "full", SelectionReason: "selection.full_required", SelectionDigest: "d", Status: "running"}))
	for name, status := range statuses {
		must(st.CreateJob(ctx, store.JobRow{ID: "run-1." + name, PipelineID: "run-1", Name: name, Kind: "test", Idempotent: true, Required: true, InputDigest: "i"}))
		must(st.SetJobStatus(ctx, "run-1."+name, status))
	}
	f.gather = job.Gatherer{Store: st, RepoID: func(string) string { return "repo-1" },
		Worktree: func(context.Context, string) (string, bool) { return f.sha, f.dirty },
		Inputs: func(context.Context, string, string) (model.InputDigests, model.InputDigests, error) {
			f.asked++
			return f.bound, f.now, f.inErr
		}}
	f.attempt = job.Attempt{RunID: "run-1", Revision: revision, Pipeline: model.PipelineRef{ID: "run-1", Name: "release", Kind: "default"}, Deps: []string{"build", "lint"}}
	return f
}

func passed() map[string]string { return map[string]string{"build": "passed", "lint": "passed"} }

func (f *runFixture) provide(t *testing.T) job.Provider {
	t.Helper()
	p, perr := f.gather.Provide(context.Background(), f.attempt)
	if perr != nil {
		t.Fatal(perr)
	}
	return p
}

func judged(t *testing.T, p job.Provider) string {
	t.Helper()
	facts, perr := p(context.Background())
	if perr != nil {
		return reasonOf(t, perr)
	}
	return reasonOf(t, job.Evaluate(facts))
}

// TestProviderReadsTheRunAndRechecksTheWorktreeOnEveryAsk: the dep outcomes come
// from the store, and a change made after the attempt was submitted (a commit,
// an edit, a lockfile change) is seen by the next admission try.
func TestProviderReadsTheRunAndRechecksTheWorktreeOnEveryAsk(t *testing.T) {
	f := newRun(t, passed())
	p := f.provide(t)
	if got := judged(t, p); got != "" {
		t.Fatalf("a clean run was refused: %s", got)
	}
	f.dirty = true
	if got := judged(t, p); got != job.DepsUnbound {
		t.Fatalf("a dirty worktree: %q", got)
	}
	f.dirty, f.sha = false, strings.Repeat("b", 40)
	if got := judged(t, p); got != job.DepsUnbound {
		t.Fatalf("a moved HEAD: %q", got)
	}
	f.sha = ""
	if got := judged(t, p); got != job.DepsUnbound {
		t.Fatalf("no HEAD: %q", got)
	}
	f.sha = revision
	f.now.Lockfiles[0].SHA256 = strings.Repeat("9", 64)
	if got := judged(t, p); got != job.DepsStale {
		t.Fatalf("a lockfile edit: %q", got)
	}
	f.now = digests()
	if got := judged(t, p); got != "" {
		t.Fatalf("back to current: %q", got)
	}
	if f.asked != 6 {
		t.Fatalf("the digester was asked %d times, want once per ask", f.asked)
	}
}

func TestProviderRefusesWhatTheStoreSaysFailed(t *testing.T) {
	for status, want := range map[string]string{"failed": job.DepsFailing, "skipped": job.DepsFailing, "cancelled": job.DepsFailing, "queued": job.DepsFailing} {
		f := newRun(t, map[string]string{"build": "passed", "lint": status})
		if got := judged(t, f.provide(t)); got != want {
			t.Errorf("lint %s: %q, want %q", status, got, want)
		}
	}
	f := newRun(t, map[string]string{"build": "passed"}) // lint does not exist in this revision
	if got := judged(t, f.provide(t)); got != job.DepsJobsMismatch {
		t.Fatalf("a removed dep: %q", got)
	}
}

func TestProviderDigesterFailuresAndAbsence(t *testing.T) {
	f := newRun(t, passed())
	f.inErr = errors.New("lockfile unreadable\nsecond line")
	p := f.provide(t)
	_, perr := p(context.Background())
	if perr == nil || perr.Reason != job.DepsStale || !strings.Contains(perr.Message, "lockfile unreadable") || strings.Contains(perr.Message, "second line") {
		t.Fatalf("digester error: %v", perr)
	}
	f = newRun(t, passed())
	f.gather.Inputs = nil
	if got := judged(t, f.provide(t)); got != job.DepsStale {
		t.Fatalf("no digester registered: %q", got)
	}
}

func TestProviderNeedsAReadableRun(t *testing.T) {
	f := newRun(t, passed())
	for name, g := range map[string]job.Gatherer{
		"no store":    {RepoID: f.gather.RepoID, Worktree: f.gather.Worktree},
		"no worktree": {Store: f.st, RepoID: f.gather.RepoID},
		"no repo id":  {Store: f.st, Worktree: f.gather.Worktree},
	} {
		if _, perr := g.Provide(context.Background(), f.attempt); perr == nil || perr.Reason != job.NoEvidence {
			t.Errorf("%s: %v", name, perr)
		}
	}
	missing := f.attempt
	missing.RunID = "run-9"
	if _, perr := f.gather.Provide(context.Background(), missing); perr == nil || perr.Reason != job.NoEvidence {
		t.Fatalf("unknown run: %v", perr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, perr := f.gather.Provide(ctx, f.attempt); perr == nil || perr.Reason != job.NoEvidence {
		t.Fatalf("unreadable store: %v", perr)
	}
}

// TestStepAsksTheProviderOnEveryTry: a job that waits for capacity is judged on
// the state of each try, not on the state at submission.
func TestStepAsksTheProviderOnEveryTry(t *testing.T) {
	f := newRun(t, passed())
	st := job.NewStash()
	st.Put("a1", f.provide(t))
	step := job.Step(st)
	view := dispatch.View{Request: dispatch.Request{AttemptID: "a1"}}
	if err := step(context.Background(), view); err != nil {
		t.Fatalf("first try: %v", err)
	}
	f.dirty = true
	err := step(context.Background(), view)
	ref, ok := err.(*dispatch.Refusal)
	if !ok || ref.Reason != "release."+job.DepsUnbound {
		t.Fatalf("second try: %#v", err)
	}
	st.Put("a2", func(context.Context) (job.Facts, *job.Error) {
		return job.Facts{}, &job.Error{Code: job.CodeNotAdmitted, Reason: job.NoEvidence, Message: "gone"}
	})
	err = step(context.Background(), dispatch.View{Request: dispatch.Request{AttemptID: "a2"}})
	if ref, ok = err.(*dispatch.Refusal); !ok || ref.Reason != "release."+job.NoEvidence {
		t.Fatalf("a provider failure must refuse: %#v", err)
	}
}

func TestGuardJudgesFirstKeepsTheProviderWhileSubmittingThenForgets(t *testing.T) {
	f := newRun(t, passed())
	st := job.NewStash()
	ran, sawDuring := false, false
	err := st.Guard(context.Background(), "a1", f.provide(t), func() error {
		ran = true
		_, sawDuring = st.ReleaseFacts("a1")
		return nil
	})
	if err != nil || !ran || !sawDuring {
		t.Fatalf("err %v ran %v during %v", err, ran, sawDuring)
	}
	if _, ok := st.ReleaseFacts("a1"); ok {
		t.Fatal("the provider outlived the submission")
	}
	boom := errors.New("queue full")
	if err = st.Guard(context.Background(), "a1", f.provide(t), func() error { return boom }); err != boom {
		t.Fatalf("next's error was rewritten: %v", err)
	}
	f.dirty = true
	ran = false
	err = st.Guard(context.Background(), "a1", f.provide(t), func() error { ran = true; return nil })
	var je *job.Error
	if !errors.As(err, &je) || je.Reason != job.DepsUnbound || ran {
		t.Fatalf("an unacceptable attempt must fail fast without submitting: %v (ran %v)", err, ran)
	}
	if _, ok := st.ReleaseFacts("a1"); ok {
		t.Fatal("a refused attempt left a provider behind")
	}
	if err = st.Guard(context.Background(), "a1", func(context.Context) (job.Facts, *job.Error) {
		return job.Facts{}, &job.Error{Code: job.CodeNotAdmitted, Reason: job.NoEvidence, Message: "gone"}
	}, func() error { ran = true; return nil }); err == nil || ran {
		t.Fatal("a provider failure must fail fast")
	}
}
