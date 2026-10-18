package evidence

// Purpose: the worktree binding (ADR 0025 X2): whether a run saw exactly the
// commit it names. `git status --porcelain` (tracked changes and untracked,
// non-ignored files) decides clean or dirty before any job starts; a dirty
// tree also carries the sha256 of `git diff HEAD --binary`.
// Inputs:  a repository root. Outputs: a Binding snapshot.
// Constraints: a failed git call is dirty (fail closed); the diff digest is
// recorded only for a dirty tree, never a bound one.

import (
	"context"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Worktree is the state of the checkout at one moment.
type Worktree struct {
	Dirty bool
	// Tracked is the digest of the tracked changes; nil when the tree is clean.
	Tracked *string
}

// ReadWorktree reads the state of root.
func ReadWorktree(ctx context.Context, c Capture, root string) Worktree {
	status, ok := git(ctx, c, root, "status", "--porcelain", "--untracked-files=normal")
	if ok && strings.TrimSpace(status) == "" {
		return Worktree{}
	}
	w := Worktree{Dirty: true}
	if diff, ok := git(ctx, c, root, "diff", "HEAD", "--binary"); ok {
		d := digestString(diff)
		w.Tracked = &d
	}
	return w
}

// Apply writes the binding of w into ev. dirty is the coordinator's own
// verdict (it also covers a tree that moved while the jobs ran, a missing
// commit and a config outside the repository); either one unbinds.
func Apply(ev *model.Evidence, w Worktree, dirty bool) {
	if !dirty && !w.Dirty {
		ev.WorktreeState, ev.TrackedChangesDigest = "clean", nil
		ev.Binding = model.Binding{State: "bound"}
		return
	}
	reason := model.BindingReason("dirty")
	ev.WorktreeState, ev.TrackedChangesDigest = "dirty", w.Tracked
	ev.Binding = model.Binding{State: "unbound", Reason: &reason}
}
