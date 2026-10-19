package run

// Purpose: wires the release job class (internal/release/job, P7-CACHE-05,
// Epic D9) into the run: registers the `release` admission step with the
// dispatcher, gathers the facts that step judges before each release attempt is
// submitted, and refuses a release job that would run without the idempotency
// key or with a deploy credential. This file does not import internal/dispatch
// (layers.yaml): the step is registered through internal/release/job.
// Inputs:  AttemptSpec (Dispatch wrapper), AttemptInfo (attempt-start hook).
// Outputs: the previous Dispatch hook's answer, or an E715 refusal.
// Constraints: V15 only, like the dispatcher; the facts come from the run's own
// store rows and git worktree, never from a file the job could write; a release
// attempt whose facts cannot be built is refused (fail closed). ReleaseInputs
// is the digester seam: until a package registers it, freshness is unknown and
// every release job is refused with deps_stale.

import (
	"context"

	compat "github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// ReleaseInputs digests the inputs of a run: bound is the state at revision,
// now the state of the working tree under root. Registered by the package that
// owns input digests (internal/evidence imports run, so run cannot call it).
var ReleaseInputs func(ctx context.Context, root, revision string) (bound, now model.InputDigests, err error)

var releaseStash = job.NewStash()

func init() {
	// A key derived anywhere else would make `runs resolve` reject the key the job received.
	if job.IdempotencyKey("a") != idempotencyKey(&model.Job{Kind: "release"}, "a") {
		panic("run: the release idempotency key drifted from internal/release/job")
	}
	if Dispatch == nil {
		panic("run: release_hook.go must initialise after dispatch_hook.go")
	}
	if err := job.Register(releaseStash); err != nil {
		panic(err)
	}
	prev := Dispatch
	Dispatch = func(ctx context.Context, s AttemptSpec) (Admitted, error) { return releaseDispatch(ctx, s, prev) }
	RegisterAttemptStart(releaseAttemptStart{})
}

// releaseFailure carries a release refusal as a run error with its code.
func releaseFailure(e *job.Error) error { return fail(e.Code, "%s: %s", e.Reason, e.Message) }

func releaseDispatch(ctx context.Context, s AttemptSpec, prev DispatchHook) (Admitted, error) {
	if !compat.V15() || s.Job == nil || s.Job.Kind != "release" {
		return prev(ctx, s)
	}
	id, _, err := attemptIDs(ctx, s)
	if err != nil {
		return Admitted{}, err
	}
	facts, ferr := releaseFacts(ctx, SessionFrom(ctx), s)
	if ferr != nil {
		return Admitted{}, releaseFailure(ferr)
	}
	if ferr = job.Evaluate(facts); ferr != nil { // fail fast; the admission step judges the same facts again
		return Admitted{}, releaseFailure(ferr)
	}
	releaseStash.Put(id, facts)
	defer releaseStash.Forget(id)
	return prev(ctx, s)
}

// releaseFacts reads the run's rows and worktree into the facts of one attempt.
func releaseFacts(ctx context.Context, sess *Session, s AttemptSpec) (job.Facts, *job.Error) {
	notReady := func(why string) (job.Facts, *job.Error) {
		return job.Facts{}, &job.Error{Code: job.CodeNotAdmitted, Reason: job.NoEvidence, Message: why}
	}
	if sess == nil || sess.Store == nil {
		return notReady("the run has no session")
	}
	row, ok, err := sess.Store.PipelineByID(ctx, sess.RunID)
	if err != nil || !ok {
		return notReady("the run's pipeline row is unavailable")
	}
	jobs, err := sess.Store.PipelineJobs(ctx, sess.RunID)
	if err != nil {
		return notReady("the run's jobs are unavailable")
	}
	state := job.RunState{RunID: sess.RunID, RepoID: shortDigest(row.Project), Revision: sess.Revision, PolicyDigest: row.PolicyDigest,
		ConfigDigest: row.InputDigest, SourceTrust: model.TrustClass(row.SourceTrust), Trigger: model.Trigger(row.Trigger),
		Selection: model.Selection{Mode: model.SelectionMode(row.SelectionMode), Reason: model.Reason(row.SelectionReason), Digest: row.SelectionDigest},
		Deps:      s.Job.Deps}
	if s.Pipeline != nil {
		state.Pipeline = model.PipelineRef{ID: sess.RunID, Name: s.Pipeline.Name, Kind: s.Pipeline.Kind}
	}
	for _, j := range jobs {
		state.Outcomes = append(state.Outcomes, job.Dep{Name: j.Name, Status: j.Status, Required: j.Required})
	}
	rev := GitRevision(ctx, row.Project, Capture(report.Capture))
	state.Clean = rev.SHA != "" && rev.SHA == sess.Revision && !rev.Dirty
	if ReleaseInputs != nil {
		bound, now, err := ReleaseInputs(ctx, row.Project, sess.Revision)
		if err != nil {
			return job.Facts{}, &job.Error{Code: job.CodeNotAdmitted, Reason: job.DepsStale, Message: "the inputs cannot be digested: " + firstLineOf(err.Error())}
		}
		state.Bound, state.Now = &bound, &now
	}
	return job.BuildFacts(state), nil
}

// releaseAttemptStart refuses a release attempt that would start without the
// idempotency key or with a deploy credential in its declaration.
type releaseAttemptStart struct{}

func (releaseAttemptStart) AttemptStart(_ context.Context, a AttemptInfo) error {
	if a.Job.Kind != "release" {
		return nil
	}
	if e := job.CheckDeclared(a.Job); e != nil {
		return releaseFailure(e)
	}
	if a.IdempotencyKey == "" {
		return releaseFailure(&job.Error{Code: job.CodeNotAdmitted, Reason: job.NoKey, Message: "a release attempt must receive " + job.KeyEnv})
	}
	return nil
}
