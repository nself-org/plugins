package run

// Purpose: publishes a release after the run (Epic CACHE D15, P7-CACHE-18): when
// a run that started a release job ends with pipeline evidence recorded as
// class `release` and result `pass`, the AfterRun hook calls the registered
// ReleasePublisher (internal/release/publish, installed by cmd; run cannot
// import it because the verifier imports internal/evidence, which imports run).
// Inputs:  every release attempt passing the Dispatch hook, local or placed on a
// node (the session's store and home, the project from the run row) and the
// RunInfo handed to AfterRun.
// Outputs: nothing on success except a note; on a failed publication an error
// that carries exit status 1, so finish.go turns it into the exit code of
// `nself ci`, prints its text (the recovery command) as a note and posts the
// commit status as a failure (the hook is a StatusGate: it runs before the status).
// Constraints: the evidence is already recorded and is never touched; a run
// without a release job, or whose release did not pass, publishes nothing; a
// missing publisher is a no-op (an embedder that wires none publishes none);
// the per-run context is dropped when AfterRun reads it; a passing release the
// hook never saw dispatched, and a publisher that panics, fail the run (exit 1)
// like any other failed publication; the publisher gets its own 10 minute
// context, not the run's spent cleanup deadline. The publisher alone
// decides TEAM behaviour: it records awaiting-signature and resolves nothing.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// ReleasePublishRequest is what the publisher needs about a finished run.
type ReleasePublishRequest struct {
	RunID, Revision string
	Store           *store.Store
	// Home is the run's state directory ("" = NSELF_CI_HOME or ~/.nself/ci).
	Home string
	// Root is the project root the run was started for.
	Root string
}

// ReleaseOutcome is what a publication reports.
type ReleaseOutcome struct {
	State, Dir, Message string
}

// ReleasePublisher is installed by cmd. It returns publish.ErrNotRelease
// (matched through ErrNothingToPublish) when the run has nothing to publish.
var ReleasePublisher func(ctx context.Context, r ReleasePublishRequest) (ReleaseOutcome, error)

// ErrNothingToPublish is what a publisher returns for a run without a passing release job.
var ErrNothingToPublish = errors.New("run: nothing to publish")

// publishBudget bounds one publication; the run's own 15 second cleanup
// context (already partly spent on the evidence and the status) is not used.
const publishBudget = 10 * time.Minute

// releasePublish is the hook; runs maps a run id to its request.
type releasePublish struct{ runs sync.Map }

func init() {
	h := &releasePublish{}
	prev := Dispatch
	// Dispatch sees every release attempt, local or placed on a node (AttemptStart
	// fires only for local ones), so the request is captured for all of them.
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) { return h.dispatch(ctx, s, prev) }
	RegisterAfterRun(h)
}

func (h *releasePublish) dispatch(ctx context.Context, s AttemptSpec, prev DispatchHook) (Admitted, error) {
	if sess := SessionFrom(ctx); sess != nil && s.Job != nil && s.Job.Kind == "release" && sess.Store != nil {
		root := ""
		if row, ok, err := sess.Store.PipelineByID(ctx, sess.RunID); err == nil && ok {
			root = row.Project
		}
		h.runs.Store(sess.RunID, ReleasePublishRequest{RunID: sess.RunID, Revision: sess.Revision, Store: sess.Store, Home: sess.Home, Root: root})
	}
	return prev(ctx, s)
}

// BeforeStatus makes the hook a StatusGate: the commit status waits for the publication.
func (*releasePublish) BeforeStatus() {}

// hasPassingRelease reports the evidence class `release`: a passing release check.
func hasPassingRelease(ev model.Evidence) bool {
	for _, c := range ev.Checks {
		if c.Kind == "release" && c.Result == "pass" {
			return true
		}
	}
	return false
}

func (h *releasePublish) AfterRun(ctx context.Context, r RunInfo) (err error) {
	v, tracked := h.runs.LoadAndDelete(r.RunID)
	if r.Result != "pass" || !hasPassingRelease(r.Evidence) || ReleasePublisher == nil {
		return nil
	}
	if !tracked { // a passing release the hook never saw dispatched must not read as published
		return &publishFailure{err: errors.New("E717 signer_failed: the release of run " + r.RunID + " was not tracked: release NOT published; run `nself ci artifacts sign " + r.RunID + "` to publish it")}
	}
	defer func() {
		if p := recover(); p != nil { // guard() would turn a panic into a note with no exit status
			err = &publishFailure{err: errors.New("E717 signer_failed: the publication stopped unexpectedly: release NOT published; run `nself ci artifacts sign " + r.RunID + "` to publish it")}
		}
	}()
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishBudget)
	defer cancel()
	out, perr := ReleasePublisher(pctx, v.(ReleasePublishRequest))
	switch {
	case errors.Is(perr, ErrNothingToPublish):
		return nil
	case perr != nil:
		return &publishFailure{err: perr}
	case out.Message != "":
		return releaseNote(out.Message)
	}
	return releaseNote("release " + out.State + ": " + out.Dir)
}

// publishFailure is a failed publication: the run's exit status becomes 1.
type publishFailure struct{ err error }

func (e *publishFailure) Error() string { return e.err.Error() }
func (e *publishFailure) Unwrap() error { return e.err }
func (e *publishFailure) ExitCode() int { return 1 }

// releaseNote is an informational result printed as a note; it changes no exit status.
type releaseNote string

func (n releaseNote) Error() string { return string(n) }
