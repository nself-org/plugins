package source

// Purpose: ClassifyLocal decides how far a local checkout is the operator's own (Epic D17).
// Inputs:  GitFacts gathered by the hook through the exec command runner.
// Outputs: a Result; owner only on provenance, never on who the author says they are.
// Constraints: pure. owner when HEAD is reachable from the default branch of the remote, or when
//          every commit the default branch does not contain sits on no ref of any remote (refs/pull/*,
//          fork remotes and FETCH_HEAD included) AND the local reflog first recorded it as created
//          here (commit, commit (amend), rebase, merge, cherry-pick; never checkout, fetch, pull).
//          Otherwise collaborator when HEAD is on a branch of the origin remote, else untrusted. A fact
//          that could not be gathered closes the path that needs it. A repository with no remote at
//          all has nothing foreign to classify, so it is the operator's own (local.no_remote).

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// ClassifyLocal classifies the checkout described by g.
func ClassifyLocal(g GitFacts) Result {
	r := Result{VisibilityZone: "private-infrastructure"}
	done := func(c model.TrustClass, reasons ...string) Result {
		r.Class = c
		r.Reasons = append(r.Reasons, reasons...)
		return r
	}
	if g.Head == "" {
		return done(untrusted, ReasonLocalNoHead)
	}
	if g.NotARepo {
		return done(owner, ReasonLocalNoGit)
	}
	if g.InProgress {
		return done(untrusted, ReasonLocalInProgress)
	}
	if g.NoRemotes && len(g.Missing) == 0 {
		return done(owner, ReasonLocalNoRemote)
	}
	if g.ReachableFromDefault && g.DefaultRemoteRef != "" {
		return done(owner, ReasonLocalReachable)
	}
	if createdHere(g) {
		return done(owner, ReasonLocalCreated)
	}
	if len(g.Missing) > 0 {
		r.Reasons = append(r.Reasons, ReasonLocalIncomplete)
	}
	if g.OnOriginRef && !g.missing(MissingRemoteRefs) {
		return done(collaborator, ReasonLocalOnOrigin)
	}
	return done(untrusted, ReasonLocalForeign)
}

// createdHere: there are commits beyond the default branch, none of them is on any remote ref, and
// the reflog first recorded each as made on this machine.
func createdHere(g GitFacts) bool {
	if (g.DefaultRemoteRef == "" && !g.NeverFetched) || len(g.OffDefault) == 0 || g.OnAnyRemoteRef {
		return false
	}
	for _, m := range []string{MissingDefaultRef, MissingRemoteRefs, MissingUnpushed, MissingReflog} {
		if g.missing(m) {
			return false
		}
	}
	unpushed := make(map[string]bool, len(g.Unpushed))
	for _, c := range g.Unpushed {
		unpushed[c.SHA] = true
	}
	for _, c := range g.OffDefault {
		if !unpushed[c.SHA] || !MadeHere(g.ReflogCreated[c.SHA]) {
			return false
		}
	}
	return true
}

// MadeHere reports whether a reflog subject records a commit created on this machine: `commit`,
// `commit (amend)`, a rebase step that rewrites a commit, `cherry-pick` and a merge. Anything that
// moves a branch to a commit someone else made is not creation: a fast-forward (of a merge, a pull, a
// cherry-pick or a rebase), `rebase (finish)` and `rebase (start)`, checkout, fetch, pull, reset.
func MadeHere(subject string) bool {
	if strings.Contains(strings.ToLower(subject), "fast-forward") {
		return false
	}
	kind, _, _ := strings.Cut(subject, ":")
	kind = strings.TrimSpace(kind)
	switch {
	case kind == "commit", strings.HasPrefix(kind, "commit ("):
		return true
	case strings.HasPrefix(kind, "rebase"):
		for _, step := range []string{"(pick)", "(reword)", "(squash)", "(fixup)", "(edit)", "(continue)"} {
			if strings.HasSuffix(kind, step) {
				return true
			}
		}
		return false
	case strings.HasPrefix(kind, "cherry-pick"):
		return true
	case kind == "merge" || strings.HasPrefix(kind, "merge "):
		return true
	}
	return false
}
