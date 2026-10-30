package watch

// Purpose: which discovered heads a personal watcher may consider, and in what order (P7-AUTO-11).
// Inputs:  the heads of every repository's scan; the ports that say whether the project is team-owned
//          and whether an isolated runner exists.
// Outputs: Order (newest default-branch head first) and gate (a skip reason, or "" to go on).
// Constraints: a fork pull request head is never run, whatever else is true; an untrusted head is never
//          run; a collaborator head runs only on an isolated runner and is otherwise reported skipped with
//          E821, never forced onto this machine; a team-owned project is skipped (team_owned) because a
//          PERSONAL process must not post for it (HOST-02).

import (
	"context"
	"sort"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// rank orders the kinds of head: the default branch, protected branches, pull requests, extra branches.
func rank(h Head) int {
	switch {
	case h.Default:
		return 0
	case h.PR == 0 && h.Protected && !h.Extra:
		return 1
	case h.PR > 0:
		return 2
	}
	return 3
}

// Order sorts heads in place: by kind, then the newest commit first (an unknown time keeps the scan's
// order, which lists pull requests by their last update).
func Order(heads []Head) {
	sort.SliceStable(heads, func(i, j int) bool {
		ri, rj := rank(heads[i]), rank(heads[j])
		if ri != rj {
			return ri < rj
		}
		a, b := heads[i].CommittedAt, heads[j].CommittedAt
		if a.IsZero() || b.IsZero() {
			return false
		}
		return a.After(b)
	})
}

// Gate is the rule for one head before any ledger read: "" lets it go on, else the skip reason and
// its code. isolated is filled in for a collaborator head that may run on an isolated runner.
func (w *Watcher) gate(ctx context.Context, h Head) (reason, code string, isolated bool) {
	if h.Fork {
		return SkipFork, "", false
	}
	if w.P.TeamOwned != nil && w.P.TeamOwned(ctx, h.Repo) {
		return SkipTeam, "", false
	}
	switch h.Trust {
	case model.TrustClass("owner"):
		return "", "", false
	case model.TrustClass("collaborator"):
		if w.P.Isolated == nil || !w.P.Isolated(ctx) {
			return SkipIsolated, CodeIsolated, false
		}
		return "", "", true
	}
	return SkipApproval, "", false
}
