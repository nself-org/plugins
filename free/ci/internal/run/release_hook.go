package run

// Purpose: wires the release job class (internal/release/job, P7-CACHE-05,
// Epic D9) into the run, as glue only: registers the `release` admission step
// with the dispatcher, submits each release attempt through the job package's
// guard (facts judged before it queues and again at every admission try), checks
// the final job environment for deploy credentials, and refuses an attempt that
// would start without the idempotency key. This file does not import
// internal/dispatch (layers.yaml): the step is registered through
// internal/release/job.
// Inputs:  AttemptSpec (Dispatch wrapper), JobSpec (isolator wrapper),
// AttemptInfo (attempt-start hook).
// Outputs: the previous hook's answer, or an E715 refusal.
// Constraints: a release job runs only under the V15 engine (the v1.4 gate has
// no release class): anywhere else it is refused. The facts come from the run's
// own store rows and git worktree, never from a file the job could write.
// A caller that supplies its own Hooks.Isolation replaces the registered one and
// with it the deploy credential check on the final environment (the in-process
// API of tests and embedders; the shipped binary uses the registered chain).
// ReleaseInputs is the digester seam: until a package registers it, freshness is
// unknown and every release job is refused with deps_stale (fail closed).

import (
	"context"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// ReleaseInputs digests the inputs of a run: bound is the state at the revision,
// now the state of the working tree under root. The package that owns input
// digests registers it (internal/evidence imports run, so run cannot call it).
var ReleaseInputs job.Inputs

// releaseStash holds the provider of every release attempt that is being submitted.
var releaseStash = job.NewStash()

func init() {
	// A key derived anywhere else would make `runs resolve` reject the key the job received.
	if job.IdempotencyKey("a") != idempotencyKey(&model.Job{Kind: "release"}, "a") {
		panic("run: the release idempotency key drifted from internal/release/job")
	}
	if Dispatch == nil || Registered.Isolation == nil {
		panic("run: release_hook.go must initialise after dispatch_hook.go and isolation_hook.go")
	}
	if err := job.Register(releaseStash); err != nil {
		panic(err)
	}
	prev := Dispatch
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) { return releaseDispatch(ctx, s, prev) }
	RegisterIsolation(releaseIsolator{next: Registered.Isolation})
	RegisterAttemptStart(releaseAttemptStart{})
}

// releaseFailure carries a release refusal as a run error with its code.
func releaseFailure(e *job.Error) error { return fail(e.Code, "%s: %s", e.Reason, e.Message) }

// releaseDispatch submits a release attempt through the guard; any other
// attempt goes straight to the previous hook.
func releaseDispatch(ctx context.Context, s AttemptSpec, prev DispatchHook) (Admitted, error) {
	if s.Job == nil || s.Job.Kind != "release" {
		return prev(ctx, s)
	}
	if !compat.V15() {
		return Admitted{}, releaseFailure(&job.Error{Code: job.CodeNotAdmitted, Reason: job.NoEvidence, Message: "a release job runs only under the V15 engine"})
	}
	id, _, err := attemptIDs(ctx, s)
	if err != nil {
		return Admitted{}, err
	}
	sess := SessionFrom(ctx)
	if sess == nil {
		return Admitted{}, releaseFailure(&job.Error{Code: job.CodeNotAdmitted, Reason: job.NoEvidence, Message: "the run has no session"})
	}
	g := job.Gatherer{Store: sess.Store, Worktree: job.GitWorktree(job.Capture(report.Capture)), Inputs: ReleaseInputs, RepoID: shortDigest}
	attempt := job.Attempt{RunID: sess.RunID, Revision: sess.Revision, Deps: s.Job.Deps}
	if s.Pipeline != nil {
		attempt.Pipeline = model.PipelineRef{ID: sess.RunID, Name: s.Pipeline.Name, Kind: s.Pipeline.Kind}
	}
	provider, perr := g.Provide(ctx, attempt)
	if perr != nil {
		return Admitted{}, releaseFailure(perr)
	}
	var adm Admitted
	err = releaseStash.Guard(ctx, id, provider, func() (e error) { adm, e = prev(ctx, s); return })
	if je, ok := err.(*job.Error); ok {
		return Admitted{}, releaseFailure(je)
	}
	return adm, err
}

// releaseIsolator refuses a release job whose final environment (declared env
// plus what the hooks added) or secrets carry a deploy credential.
type releaseIsolator struct{ next ciexec.Isolator }

func (r releaseIsolator) Prepare(ctx context.Context, s *ciexec.JobSpec) error {
	if s.Job.Kind == "release" {
		if e := job.CheckDeclared(s.Job); e != nil {
			return releaseFailure(e)
		}
	}
	return r.next.Prepare(ctx, s)
}

// releaseAttemptStart refuses a release attempt that would start without the
// idempotency key (prepare.go exports it for at-most-once jobs).
type releaseAttemptStart struct{}

func (releaseAttemptStart) AttemptStart(_ context.Context, a AttemptInfo) error {
	if a.Job.Kind == "release" && a.IdempotencyKey == "" {
		return releaseFailure(&job.Error{Code: job.CodeNotAdmitted, Reason: job.NoKey, Message: "a release attempt must receive " + job.KeyEnv})
	}
	return nil
}
