package job

// Purpose: the coordinator side of the release gate: read what the run knows
// (the stored outcomes of the release job's deps, the worktree, the inputs
// digester) and hand the admission step a Provider that rebuilds the Facts on
// every ask.
// Inputs:  the run's store, a worktree probe and an inputs digester, plus the
// attempt's run id, revision and deps.
// Outputs: a Provider, or an E715 *Error when the run cannot be read.
// Constraints: the dep outcomes are read once, when the attempt is submitted (a
// job that passed stays passed); the worktree probe and the digester are asked
// again on every Provider call, so the checks that can go stale while the
// attempt waits for capacity are never cached. A missing digester means
// freshness is unknown, which refuses (deps_stale). The store is never touched
// from inside the Provider (the admission step runs in the writer transaction).

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Worktree says which commit the checkout under root is at and whether it has
// uncommitted changes (tracked or untracked).
type Worktree func(ctx context.Context, root string) (sha string, dirty bool)

// Inputs digests the inputs of the revision (bound) and of the working tree
// (now) under root.
type Inputs func(ctx context.Context, root, revision string) (bound, now model.InputDigests, err error)

// Gatherer reads the run.
type Gatherer struct {
	Store    *store.Store
	Worktree Worktree
	Inputs   Inputs // nil: freshness unknown
	// RepoID derives the repository id evidence carries from the project root.
	RepoID func(project string) string
}

// Attempt names the release attempt to gather facts for.
type Attempt struct {
	RunID, Revision string
	Pipeline        model.PipelineRef
	Deps            []string
}

func notReady(reason, format string, a ...any) *Error {
	return refuse(CodeNotAdmitted, reason, format, a...)
}

// Provide reads the run's stored rows once and returns the Provider for a.
func (g Gatherer) Provide(ctx context.Context, a Attempt) (Provider, *Error) {
	if g.Store == nil || g.Worktree == nil || g.RepoID == nil {
		return nil, notReady(NoEvidence, "the run has no store or worktree probe")
	}
	row, ok, err := g.Store.PipelineByID(ctx, a.RunID)
	if err != nil || !ok {
		return nil, notReady(NoEvidence, "the run's pipeline row is unavailable")
	}
	jobs, err := g.Store.PipelineJobs(ctx, a.RunID)
	if err != nil {
		return nil, notReady(NoEvidence, "the run's jobs are unavailable")
	}
	state := RunState{RunID: a.RunID, RepoID: g.RepoID(row.Project), Revision: a.Revision, PolicyDigest: row.PolicyDigest,
		ConfigDigest: row.InputDigest, SourceTrust: model.TrustClass(row.SourceTrust), Trigger: model.Trigger(row.Trigger),
		Selection: model.Selection{Mode: model.SelectionMode(row.SelectionMode), Reason: model.Reason(row.SelectionReason), Digest: row.SelectionDigest},
		Pipeline:  a.Pipeline, Deps: a.Deps}
	for _, j := range jobs {
		state.Outcomes = append(state.Outcomes, Dep{Name: j.Name, Status: j.Status, Required: j.Required})
	}
	return func(ctx context.Context) (Facts, *Error) {
		s := state
		sha, dirty := g.Worktree(ctx, row.Project)
		s.Clean = sha != "" && sha == a.Revision && !dirty
		if g.Inputs != nil {
			bound, now, err := g.Inputs(ctx, row.Project, a.Revision)
			if err != nil {
				return Facts{}, notReady(DepsStale, "the inputs cannot be digested: %s", firstLine(err.Error()))
			}
			s.Bound, s.Now = &bound, &now
		}
		return BuildFacts(s), nil
	}, nil
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
