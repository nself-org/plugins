package job

// Purpose: the `release` admission step (Epic D9). A `kind: release` job is
// admitted only when attest.GateAccept accepts the run's evidence of the job's
// deps under the release-admission gate built here. authz.Promotable is not
// used: it refuses non-release evidence by design (D16 uses it on the
// published release's evidence), and this package does not import authz.
// Inputs:  a Provider of Facts per release attempt (evidence of the deps, the
// run's protected identity and the digests of the inputs now).
// Outputs: nil (admit) or a *dispatch.Refusal naming the E715 reason.
// Constraints: the step runs inside the admission transaction and calls no
// Store method (a Provider may read the worktree, never the store); it asks the
// Source's Provider for the attempt id on every try. The gate is
// built from Facts.Repo/Revision/PolicyDigest/Deps, never from the evidence
// being judged. The only submission path (run's Dispatch wrapper) refuses to
// submit a release attempt whose facts it cannot build, so no release attempt
// reaches this step without facts; an attempt with none is not a release one.

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/dispatch"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// StepName is the admission step's registry name.
const StepName = "release"

// Facts is what the admission of one release attempt judges.
type Facts struct {
	// Evidence is the coordinator-recorded evidence of the run's deps.
	Evidence model.Evidence
	// RepoID, Revision and PolicyDigest come from the run's own pipeline row.
	RepoID, Revision, PolicyDigest string
	// Deps are the release job's deps: the jobs the evidence must show passing.
	Deps []string
	// Current holds the digests of the inputs now; nil means unknown (stale).
	Current *model.InputDigests
}

// Gate builds the release-admission gate: owner or internal source, default
// class, no self-reported evidence, full selection, exact revision, current.
func Gate(f Facts) attest.Gate {
	deps := append([]string(nil), f.Deps...)
	sort.Strings(deps)
	return attest.Gate{RepoID: f.RepoID, Revision: f.Revision, PolicyDigest: f.PolicyDigest, RequiredJobs: deps,
		MinTrust: "internal", Class: attest.ClassDefault, AllowSelfReported: false, FullSelection: true, Current: f.Current}
}

// reasonOf maps an attest reason to its E715 reason.
var reasonOf = map[attest.Reason]string{
	attest.ReasonNotPassing: DepsFailing, attest.ReasonStale: DepsStale, attest.ReasonUnbound: DepsUnbound,
	attest.ReasonJobsMismatch: DepsJobsMismatch, attest.ReasonSelfReportedNotAdmitted: SelfReported, attest.ReasonAdhoc: Adhoc,
	attest.ReasonTrustBelowMin: Trust, attest.ReasonRepoMismatch: DepsMismatch, attest.ReasonRevisionMismatch: DepsMismatch,
	attest.ReasonPolicyMismatch: DepsMismatch, attest.ReasonSelectionPartial: SelectionPartial,
	attest.ReasonInvalid: DepsInvalid, attest.ReasonSignatureUnverifiable: DepsInvalid,
}

// Evaluate judges the facts. nil admits; otherwise the E715 carries the first
// reason (attest's fixed order) and lists all of them.
func Evaluate(f Facts) *Error {
	reasons := attest.GateAccept(f.Evidence, Gate(f))
	if len(reasons) == 0 {
		return nil
	}
	all := make([]string, len(reasons))
	for i, r := range reasons {
		all[i] = string(r)
	}
	first, ok := reasonOf[reasons[0]]
	if !ok {
		first = DepsInvalid // an unmapped reason never admits
	}
	return refuse(CodeNotAdmitted, first, "the evidence of the release job's deps is not acceptable (%s)", strings.Join(all, ", "))
}

// Provider builds the Facts of one release attempt at the moment it is asked:
// the admission step asks again on every try, so a change made while the
// attempt waits for capacity is seen (freshness and binding are never cached).
type Provider func(ctx context.Context) (Facts, *Error)

// Static is a Provider that always answers f.
func Static(f Facts) Provider { return func(context.Context) (Facts, *Error) { return f, nil } }

// Source finds the Provider of a release attempt; ok is false for any other attempt.
type Source interface {
	ReleaseFacts(attemptID string) (p Provider, ok bool)
}

// judge asks p and evaluates its answer.
func judge(ctx context.Context, p Provider) *Error {
	f, perr := p(ctx)
	if perr != nil {
		return perr
	}
	return Evaluate(f)
}

// Step is the admission step over src.
func Step(src Source) dispatch.Step {
	return func(ctx context.Context, v dispatch.View) error {
		p, ok := src.ReleaseFacts(v.Request.AttemptID)
		if !ok {
			return nil
		}
		if e := judge(ctx, p); e != nil {
			return &dispatch.Refusal{Reason: model.PlacementReason("release." + e.Reason), Detail: e.Error()}
		}
		return nil
	}
}

// Register adds the step to the dispatcher; call it from init().
func Register(src Source) error { return dispatch.RegisterStep(StepName, Step(src)) }

// Stash holds the Providers of release attempts between the coordinator's
// preparation and the admission transaction. Safe for concurrent use.
type Stash struct {
	mu sync.Mutex
	m  map[string]Provider
}

// NewStash returns an empty stash.
func NewStash() *Stash { return &Stash{m: map[string]Provider{}} }

// Put records the provider of one attempt.
func (s *Stash) Put(attemptID string, p Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[attemptID] = p
}

// Forget drops the provider of one attempt.
func (s *Stash) Forget(attemptID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, attemptID)
}

// ReleaseFacts implements Source.
func (s *Stash) ReleaseFacts(attemptID string) (Provider, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.m[attemptID]
	return p, ok
}

// Guard submits one release attempt: it judges p first (an unacceptable
// attempt fails fast with the E715 *Error and never queues), keeps p in the
// stash while next runs (next blocks until the attempt is admitted, and the
// admission step judges p again), then forgets it.
func (s *Stash) Guard(ctx context.Context, attemptID string, p Provider, next func() error) error {
	if e := judge(ctx, p); e != nil {
		return e
	}
	s.Put(attemptID, p)
	defer s.Forget(attemptID)
	return next()
}
