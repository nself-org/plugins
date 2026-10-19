package run

// Purpose: the run-side slots the AUTO Epic fills (P7-AUTO-27 adds the slots
// once so no later Ticket edits hooks.go, finish.go or coordinator.go).
//   PublishGate   second publisher-side veto (P7-AUTO-15); composes with the
//                 TRUST-11 attest Publisher, never replaces it
//   PublishDetail receives run id, gate table and failing diagnostics next to
//                 the status (P7-AUTO-22)
//   OffloadHook   turns a dirty tree into a revision a node can fetch (P7-AUTO-13)
//   RaceHook      lets sibling attempts of one job race (P7-AUTO-14)
//   NotifyHook    tells an operator a run ended (P7-AUTO-18)
// Inputs:  registrations from init() of other packages, or Hooks.Auto in tests.
// Outputs: nothing when no slot is filled: behaviour and goldens are unchanged.
// Constraints: a hook error is a note, never a changed verdict, except Offload
// (refuses the run before anything is persisted) and a PublishGate veto
// (nothing is posted). Every call is guarded against panics.

import (
	"context"
	"encoding/json"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// RunValues are per-run policy values (PolicyRequest.Run). Keys are policy
// keys; a value may only tighten what the project and operator scopes allow.
type RunValues = map[string]json.RawMessage

// PublishGate vetoes the post of a status. A non-nil error means "post nothing";
// its text is shown after "nothing posted". It sees the status the Publisher built.
type PublishGate interface {
	Gate(ctx context.Context, r PublishRequest, s report.Status) error
}

// PublishDetailRequest is what a detail publisher receives next to the status: the
// run id, the gate table (Evidence.Checks) and the failing diagnostics (the checks'
// Diagnostics). Evidence is the PUBLISHED document, with the publication profile of
// the repository applied (TRUST-11), exactly as for the status; the unpublished
// evidence and job tails never reach this slot.
type PublishDetailRequest struct {
	Owner, Repo, RunID, Revision string
	Status                       report.Status
	Evidence                     model.Evidence
	Projection                   model.ProjectionResult
}

// PublishDetail publishes richer output after the status was delivered.
type PublishDetail interface {
	Detail(ctx context.Context, r PublishDetailRequest) error
}

// OffloadRequest asks the offload hook whether this run's tree needs a snapshot.
type OffloadRequest struct {
	Opts Options
	Rev  Revision
}

// OffloadResult replaces the run's revision when Rev.SHA is set (a snapshot
// commit of a dirty tree). Release runs when the run ends; it may be nil.
type OffloadResult struct {
	Rev     Revision
	Release func()
}

// OffloadHook runs once per run before anything is persisted. An error refuses the run.
type OffloadHook interface {
	Offload(ctx context.Context, r OffloadRequest) (OffloadResult, error)
}

// RaceRequest describes a persisted run just before its first job starts.
type RaceRequest struct {
	RunID string
	Opts  Options
	Plan  *plan.Plan
}

// RaceHook is called once per run after it is persisted and before the jobs
// start. It has no error: a race that cannot start degrades to one attempt.
type RaceHook interface {
	Race(ctx context.Context, r RaceRequest)
}

// NotifyRequest describes a finished run: the status line the run printed (it
// says whether a status was posted) and the notes collected so far.
type NotifyRequest struct {
	Run    RunInfo
	Status string
	Notes  []string
}

// NotifyHook is called once per finished run (one that was persisted and concluded)
// after the AfterRun hooks and finalizers. Runs refused before that point, such as an
// offload refusal, return an error to the caller and notify nothing. Notes is a copy.
type NotifyHook interface {
	Notify(ctx context.Context, r NotifyRequest) error
}

// AutoHooks groups the AUTO slots inside Hooks. A nil or empty field is a no-op.
type AutoHooks struct {
	Gates   []PublishGate
	Details []PublishDetail
	Offload OffloadHook
	Race    RaceHook
	Notify  NotifyHook
}

// RegisterPublishGate adds a veto; every registered gate must allow a post.
func RegisterPublishGate(g PublishGate) { Registered.Auto.Gates = append(Registered.Auto.Gates, g) }

// RegisterPublishDetail adds a detail publisher.
func RegisterPublishDetail(d PublishDetail) {
	Registered.Auto.Details = append(Registered.Auto.Details, d)
}

// RegisterOffload sets the offload hook (single slot).
func RegisterOffload(h OffloadHook) { Registered.Auto.Offload = h }

// RegisterRace sets the race hook (single slot).
func RegisterRace(h RaceHook) { Registered.Auto.Race = h }

// RaceRegistered says whether a race hook is installed (`--where all` needs one, P7-AUTO-06).
func RaceRegistered() bool { return Registered.Auto.Race != nil }

// RegisterNotify sets the notify hook (single slot).
func RegisterNotify(h NotifyHook) { Registered.Auto.Notify = h }

// resolved fills nil single slots from r and appends r's lists after h's.
func (h AutoHooks) resolved(r AutoHooks) AutoHooks {
	out := h
	out.Gates = append(append([]PublishGate(nil), h.Gates...), r.Gates...)
	out.Details = append(append([]PublishDetail(nil), h.Details...), r.Details...)
	if out.Offload == nil {
		out.Offload = r.Offload
	}
	if out.Race == nil {
		out.Race = r.Race
	}
	if out.Notify == nil {
		out.Notify = r.Notify
	}
	return out
}

// offload runs the offload hook; the returned release is never nil.
func (c *Coordinator) offload(ctx context.Context, h Hooks, rev Revision) (Revision, func(), error) {
	if h.Auto.Offload == nil {
		return rev, func() {}, nil
	}
	var res OffloadResult
	err := guard("offload hook", func() (e error) {
		res, e = h.Auto.Offload.Offload(ctx, OffloadRequest{Opts: c.Opts, Rev: rev})
		return
	})
	if err != nil {
		return rev, func() {}, err
	}
	if res.Rev.SHA != "" {
		rev = res.Rev
	}
	if res.Release == nil {
		res.Release = func() {}
	}
	return rev, res.Release, nil
}

// race calls the race hook, if any.
func (c *Coordinator) race(ctx context.Context, h Hooks, runID string) {
	if h.Auto.Race == nil {
		return
	}
	_ = guard("race hook", func() error { h.Auto.Race.Race(ctx, RaceRequest{RunID: runID, Opts: c.Opts, Plan: c.Plan}); return nil })
}

// publishGates returns the first veto of the registered gates, or nil.
func publishGates(ctx context.Context, h Hooks, r PublishRequest, s report.Status) error {
	for _, g := range h.Auto.Gates {
		if err := guard("publish gate", func() error { return g.Gate(ctx, r, s) }); err != nil {
			return err
		}
	}
	return nil
}

// publishDetails calls every detail publisher after a status was delivered, with
// the evidence the publication profile leaves.
func publishDetails(ctx context.Context, h Hooks, r PublishRequest, s report.Status) []string {
	if len(h.Auto.Details) == 0 {
		return nil
	}
	published, _, _ := publication(r)
	proj := r.Projection
	if len(published.Checks) > 0 {
		proj = model.Projection(published)
	}
	req := PublishDetailRequest{Owner: r.Owner, Repo: r.Repo, RunID: r.Run.RunID, Revision: r.Run.Revision, Status: s, Evidence: published, Projection: proj}
	var notes []string
	for _, d := range h.Auto.Details {
		if err := guard("publish detail", func() error { return d.Detail(ctx, req) }); err != nil {
			notes = append(notes, "publish detail: "+firstLineOf(err.Error()))
		}
	}
	return notes
}

// notifyRun calls the notify hook, if any, and returns the notes it produced.
func notifyRun(ctx context.Context, h Hooks, req NotifyRequest) []string {
	if h.Auto.Notify == nil {
		return nil
	}
	if err := guard("notify hook", func() error { return h.Auto.Notify.Notify(ctx, req) }); err != nil {
		return []string{"notify hook: " + firstLineOf(err.Error())}
	}
	return nil
}
