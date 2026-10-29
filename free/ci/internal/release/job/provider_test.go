package job_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// runFixture is a stored run whose build and lint jobs passed.
type runFixture struct {
	mu      sync.Mutex
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

func (f *runFixture) setDirty(v bool)      { f.mu.Lock(); f.dirty = v; f.mu.Unlock() }
func (f *runFixture) setInputsErr(e error) { f.mu.Lock(); f.inErr = e; f.mu.Unlock() }
func (f *runFixture) askedNow() int        { f.mu.Lock(); defer f.mu.Unlock(); return f.asked }

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
		Worktree: func(context.Context, string) (string, bool) { f.mu.Lock(); defer f.mu.Unlock(); return f.sha, f.dirty },
		Inputs: func(context.Context, string, string) (model.InputDigests, model.InputDigests, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
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

// await polls until cond holds (the refresher runs on its own clock).
func await(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func stepFor(st *job.Stash, id string) error {
	return job.Step(st)(context.Background(), dispatch.View{Request: dispatch.Request{AttemptID: id, Local: true}})
}

// TestGuardKeepsTheFactsFreshWhileTheAttemptWaits: a change made after the
// attempt was submitted (an edit, a moved HEAD) is seen by a later admission
// try, and the probes run outside the step, not in it.
func TestGuardKeepsTheFactsFreshWhileTheAttemptWaits(t *testing.T) {
	defer job.SetTiming(5*time.Millisecond, time.Minute)()
	f := newRun(t, passed())
	st := job.NewStash()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
		if err != nil {
			t.Error(err)
		}
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	if err := stepFor(st, "a1"); err != nil {
		t.Fatalf("first try: %v", err)
	}
	asked := f.askedNow()
	if err := stepFor(st, "a1"); err != nil || f.askedNow() != asked {
		t.Fatalf("the step must not run the digester itself (asked %d -> %d): %v", asked, f.askedNow(), err)
	}
	f.setDirty(true)
	await(t, "the refresh to see the edit", func() bool {
		ref, ok := stepFor(st, "a1").(*dispatch.Refusal)
		return ok && ref.Reason == "release."+job.DepsUnbound
	})
	f.setDirty(false)
	await(t, "the refresh to see the tree clean again", func() bool { return stepFor(st, "a1") == nil })
	close(release)
	wg.Wait()
	if _, _, ok := st.ReleaseFacts("a1"); ok {
		t.Fatal("the facts outlived the submission")
	}
}

// TestFactsExpireWhenTheRefresherStops: facts older than maxAge refuse.
func TestFactsExpireWhenTheRefresherStops(t *testing.T) {
	defer job.SetTiming(time.Hour, 20*time.Millisecond)()
	f := newRun(t, passed())
	st := job.NewStash()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	await(t, "the facts to expire", func() bool {
		ref, ok := stepFor(st, "a1").(*dispatch.Refusal)
		return ok && ref.Reason == "release."+job.FactsExpired
	})
	close(release)
	wg.Wait()
}

func TestStepRefusesWhatTheLastRefreshFailed(t *testing.T) {
	defer job.SetTiming(5*time.Millisecond, time.Minute)()
	f := newRun(t, passed())
	st := job.NewStash()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	f.setInputsErr(errors.New("lockfile unreadable"))
	await(t, "the refresh to fail", func() bool {
		ref, ok := stepFor(st, "a1").(*dispatch.Refusal)
		return ok && ref.Reason == "release."+job.DepsStale
	})
	close(release)
	wg.Wait()
}

func TestGuardJudgesFirstSubmitsThenForgets(t *testing.T) {
	f := newRun(t, passed())
	st := job.NewStash()
	ran, sawDuring := false, false
	err := st.Guard(context.Background(), "a1", f.provide(t), func() error {
		ran = true
		_, _, sawDuring = st.ReleaseFacts("a1")
		return nil
	})
	if err != nil || !ran || !sawDuring {
		t.Fatalf("err %v ran %v during %v", err, ran, sawDuring)
	}
	if _, _, ok := st.ReleaseFacts("a1"); ok {
		t.Fatal("the facts outlived the submission")
	}
	boom := errors.New("queue full")
	if err = st.Guard(context.Background(), "a1", f.provide(t), func() error { return boom }); err != boom {
		t.Fatalf("next's error was rewritten: %v", err)
	}
	f.setDirty(true)
	ran = false
	err = st.Guard(context.Background(), "a1", f.provide(t), func() error { ran = true; return nil })
	var je *job.Error
	if !errors.As(err, &je) || je.Reason != job.DepsUnbound || ran {
		t.Fatalf("an unacceptable attempt must fail fast without submitting: %v (ran %v)", err, ran)
	}
	if _, _, ok := st.ReleaseFacts("a1"); ok {
		t.Fatal("a refused attempt left facts behind")
	}
	failing := func(context.Context) (job.Facts, *job.Error) {
		return job.Facts{}, &job.Error{Code: job.CodeNotAdmitted, Reason: job.NoEvidence, Message: "gone"}
	}
	if err = st.Guard(context.Background(), "a1", failing, func() error { ran = true; return nil }); err == nil || ran {
		t.Fatal("a provider failure must fail fast")
	}
}

// TestFactsExpireAfterExactlyMaxAge: older than maxAge refuses, exactly maxAge does not.
func TestFactsExpireAfterExactlyMaxAge(t *testing.T) {
	defer job.SetTiming(time.Hour, 10*time.Second)()
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := t0
	defer job.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return clock })()
	setClock := func(d time.Duration) { mu.Lock(); clock = t0.Add(d); mu.Unlock() }
	f := newRun(t, passed())
	st := job.NewStash()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	setClock(10 * time.Second)
	if _, perr, _ := st.ReleaseFacts("a1"); perr != nil {
		t.Fatalf("facts exactly maxAge old expired: %v", perr)
	}
	setClock(10*time.Second + time.Nanosecond)
	_, perr, ok := st.ReleaseFacts("a1")
	if !ok || perr == nil || perr.Reason != job.FactsExpired || !strings.Contains(perr.Message, "10s ago") {
		t.Fatalf("facts older than maxAge: %v", perr)
	}
	close(release)
	wg.Wait()
}

// TestGuardRefusesASecondSubmissionOfOneAttempt: the second submission fails
// without touching the first one's facts.
func TestGuardRefusesASecondSubmissionOfOneAttempt(t *testing.T) {
	f := newRun(t, passed())
	st := job.NewStash()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	ran := false
	err := st.Guard(context.Background(), "a1", f.provide(t), func() error { ran = true; return nil })
	var je *job.Error
	if !errors.As(err, &je) || je.Reason != job.Duplicate || ran {
		t.Fatalf("second submission: %v (ran %v)", err, ran)
	}
	if _, _, ok := st.ReleaseFacts("a1"); !ok {
		t.Fatal("the loser's failure deleted the first submission's facts")
	}
	close(release)
	wg.Wait()
	if _, _, ok := st.ReleaseFacts("a1"); ok {
		t.Fatal("the facts outlived the submission")
	}
}

// TestSlowProviderCallsDoNotFreshenOldObservations: the age counts from when the
// provider was asked.
func TestSlowProviderCallsDoNotFreshenOldObservations(t *testing.T) {
	defer job.SetTiming(5*time.Millisecond, 10*time.Second)()
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := t0
	defer job.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return clock })()
	f := newRun(t, passed())
	f.gather.Inputs = func(context.Context, string, string) (model.InputDigests, model.InputDigests, error) {
		mu.Lock()
		defer mu.Unlock()
		clock = clock.Add(time.Minute) // the digester is slow: the clock jumps while it runs
		return f.bound, f.now, nil
	}
	st := job.NewStash()
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	await(t, "an expired observation", func() bool {
		_, perr, _ := st.ReleaseFacts("a1")
		return perr != nil && perr.Reason == job.FactsExpired
	})
	close(release)
	wg.Wait()
}

// TestProviderCarriesTheStartOfRunState (D-1075): a run that began dirty, on a
// config outside the commit or on a --job subset is refused even though the
// worktree probe answers clean and at the revision when the release job asks.
func TestProviderCarriesTheStartOfRunState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*job.Attempt)
		want   string
	}{
		{"started clean, full run", func(*job.Attempt) {}, ""},
		{"started on a dirty tree", func(a *job.Attempt) { a.Dirty = true }, job.DepsUnbound},
		{"config outside the commit", func(a *job.Attempt) { a.ConfigOutside = true }, job.DepsUnbound},
		{"--job subset", func(a *job.Attempt) { a.Subset = true }, job.Adhoc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRun(t, passed())
			tc.mutate(&f.attempt)
			if got := judged(t, f.provide(t)); got != tc.want {
				t.Fatalf("reason %q, want %q", got, tc.want)
			}
		})
	}
	f := newRun(t, passed())
	f.attempt.Subset = true
	facts, perr := f.provide(t)(context.Background())
	if perr != nil {
		t.Fatal(perr)
	}
	if e := job.Evaluate(facts); e == nil || !strings.Contains(e.Message, "evidence.selection_partial") || !strings.Contains(e.Message, "evidence.adhoc") {
		t.Fatalf("a subset run must be adhoc and partial: %v", e)
	}
}

// TestGuardRefusesWhenSignerIsRevokedBetweenGuardAndAdmit: a revocation that
// commits after Guard accepted the facts but before the admission step reads
// them must refuse with signer_revoked (regression for cached-decision race).
func TestGuardRefusesWhenSignerIsRevokedBetweenGuardAndAdmit(t *testing.T) {
	defer job.SetTiming(5*time.Millisecond, time.Minute)()
	f := newRun(t, passed())
	st := job.NewStash()
	keyID := "ci-node-abcdef0123456789"
	revoked := false
	st.Revoked = func(id string) (bool, error) { return revoked && id == keyID, nil }
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil })
		if err != nil {
			t.Error(err)
		}
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	// Simulate the evidence carrying a signer key (e.g. from a remote runner)
	// without re-running Evaluate, which would reject coordinator-recorded
	// evidence with a KeyID set.
	job.SetEntryKeyID(st, "a1", keyID)
	if err := stepFor(st, "a1"); err != nil {
		t.Fatalf("admitted before revocation: %v", err)
	}
	revoked = true
	await(t, "the revocation to be seen", func() bool {
		ref, ok := stepFor(st, "a1").(*dispatch.Refusal)
		return ok && ref.Reason == "release."+job.SignerRevoked
	})
	close(release)
	wg.Wait()
}

// TestGuardRefusesWhenTheRevocationLedgerFails: a ledger that cannot answer must
// not read as "not revoked"; the cached approval is refused (fail closed).
func TestGuardRefusesWhenTheRevocationLedgerFails(t *testing.T) {
	defer job.SetTiming(5*time.Millisecond, time.Minute)()
	f := newRun(t, passed())
	st := job.NewStash()
	keyID := "ci-node-abcdef0123456789"
	var down atomic.Bool
	st.Revoked = func(string) (bool, error) {
		if down.Load() {
			return false, errors.New("ledger unavailable")
		}
		return false, nil
	}
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := st.Guard(context.Background(), "a1", f.provide(t), func() error { <-release; return nil }); err != nil {
			t.Error(err)
		}
	}()
	await(t, "the attempt to queue", func() bool { _, _, ok := st.ReleaseFacts("a1"); return ok })
	job.SetEntryKeyID(st, "a1", keyID)
	if err := stepFor(st, "a1"); err != nil {
		t.Fatalf("admitted before the ledger failed: %v", err)
	}
	down.Store(true)
	ref, ok := stepFor(st, "a1").(*dispatch.Refusal)
	if !ok || ref.Reason != "release."+job.SignerUnverified {
		t.Fatalf("a failing ledger must refuse with signer_unverified: %v", ref)
	}
	close(release)
	wg.Wait()
}
