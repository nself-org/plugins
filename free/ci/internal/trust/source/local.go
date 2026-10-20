package source

// Purpose: ClassifyLocal decides how far a local checkout is the operator's own (Epic D17).
// Inputs:  GitFacts gathered by the hook through the exec command runner.
// Outputs: a Result; owner only on provenance, never on who the author says they are.
// Constraints: pure. owner when HEAD is reachable from the default branch of the remote, or when
//          the local reflogs first recorded every commit the default branch does not contain as created
//          here (commit, commit (amend), rebase, merge, cherry-pick; never checkout, fetch, pull, a
//          fast-forward). A pushed commit of yours still counts: D17 asks for "on no ref of any remote",
//          but an arrival entry is the proof of a stranger's commit and the push test made `nself ci`
//          refuse your own pushed branch (lead ruling, TRUST EPIC Invalidations 2026-10-19).
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

// createdHere: there are commits beyond the default branch and the reflogs first recorded every one as
// made on this machine. Being pushed does not change that: a pushed commit of yours still has its
// `commit` entry, while a stranger's commit you fetched, pulled or checked out has an arrival entry.
func createdHere(g GitFacts) bool {
	if (g.DefaultRemoteRef == "" && !g.NeverFetched) || len(g.OffDefault) == 0 {
		return false
	}
	for _, m := range []string{MissingDefaultRef, MissingRemoteRefs, MissingCommits, MissingReflog} {
		if g.missing(m) {
			return false
		}
	}
	for _, c := range g.OffDefault {
		if !MadeHere(g.ReflogCreated[c.SHA]) {
			return false
		}
	}
	return true
}

// MadeHere reports whether a reflog subject records a commit created on this machine: `commit`,
// `commit (amend)`, a rebase step that rewrites a commit (also under `pull --rebase`), `cherry-pick`,
// `revert` and a merge commit (also under `pull`). Anything that moves a branch to a commit someone
// else made is not creation: a fast-forward (of a merge, a pull, a cherry-pick or a rebase),
// `rebase (finish)` and `rebase (start)`, checkout, fetch, reset.
func MadeHere(subject string) bool {
	if strings.Contains(strings.ToLower(subject), "fast-forward") {
		return false
	}
	kind, _, _ := strings.Cut(subject, ":")
	kind = strings.TrimSpace(kind)
	switch {
	case kind == "commit", strings.HasPrefix(kind, "commit ("):
		return true
	case strings.HasPrefix(kind, "rebase"), strings.HasPrefix(kind, "pull"):
		for _, step := range []string{"(pick)", "(reword)", "(squash)", "(fixup)", "(edit)", "(continue)"} {
			if strings.HasSuffix(kind, step) {
				return true
			}
		}
		return strings.Contains(subject, "Merge made by")
	case strings.HasPrefix(kind, "cherry-pick"), kind == "revert":
		return true
	case kind == "merge" || strings.HasPrefix(kind, "merge "):
		return true
	}
	return false
}
