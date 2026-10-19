package run

// Purpose: publishes a release after the run (Epic CACHE D15, P7-CACHE-18): when
// a run that started a release job ends with pipeline evidence recorded as
// class `release` and result `pass`, the AfterRun hook calls the registered
// ReleasePublisher (internal/release/publish, installed by cmd; run cannot
// import it because the verifier imports internal/evidence, which imports run).
// Inputs:  AttemptInfo of a release attempt (store, home, project root) and the
// RunInfo handed to AfterRun.
// Outputs: nothing on success except a note; on a failed publication an error
// that carries exit status 1, so finish.go turns it into the exit code of
// `nself ci` and prints its text (the recovery command) as a note.
// Constraints: the evidence is already recorded and is never touched; a run
// without a release job, or whose release did not pass, publishes nothing; a
// missing publisher is a no-op (an embedder that wires none publishes none);
// the per-run context is dropped when AfterRun reads it. The publisher alone
// decides TEAM behaviour: it records awaiting-signature and resolves nothing.

import (
	"context"
	"errors"
	"sync"

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

// releasePublish is the hook; runs maps a run id to its request.
type releasePublish struct{ runs sync.Map }

func init() {
	h := &releasePublish{}
	RegisterAttemptStart(h)
	RegisterAfterRun(h)
}

func (h *releasePublish) AttemptStart(_ context.Context, a AttemptInfo) error {
	if a.Job.Kind == "release" && a.Store != nil {
		h.runs.Store(a.RunID, ReleasePublishRequest{RunID: a.RunID, Revision: a.Revision, Store: a.Store, Home: a.Home, Root: a.Root})
	}
	return nil
}

// hasPassingRelease reports the evidence class `release`: a passing release check.
func hasPassingRelease(ev model.Evidence) bool {
	for _, c := range ev.Checks {
		if c.Kind == "release" && c.Result == "pass" {
			return true
		}
	}
	return false
}

func (h *releasePublish) AfterRun(ctx context.Context, r RunInfo) error {
	v, ok := h.runs.LoadAndDelete(r.RunID)
	if !ok || r.Result != "pass" || !hasPassingRelease(r.Evidence) || ReleasePublisher == nil {
		return nil
	}
	out, err := ReleasePublisher(ctx, v.(ReleasePublishRequest))
	switch {
	case errors.Is(err, ErrNothingToPublish):
		return nil
	case err != nil:
		return &publishFailure{err: err}
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
