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
	"os"
	"path/filepath"
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
	// The status and the hidden-edit listing are independent funnel spawns: side by side they cost
	// the slower one, not both (D-1008). A dirty tree throws the listing away.
	hidden := make(chan bool, 1)
	go func() { hidden <- hiddenEdits(ctx, c, root) }()
	status, ok := git(ctx, c, root, "status", "--porcelain", "--untracked-files=normal")
	if hiddenEdit := <-hidden; ok && strings.TrimSpace(status) == "" && !hiddenEdit {
		return Worktree{}
	}
	w := Worktree{Dirty: true}
	if diff, ok := git(ctx, c, root, "diff", "HEAD", "--binary"); ok {
		d := digestString(diff)
		w.Tracked = &d
	}
	return w
}

// hiddenEdits reports tracked files git was told not to look at: an
// assume-unchanged entry, or a skip-worktree entry whose file is present (a
// sparse checkout leaves excluded paths absent, which hides nothing). `status`
// is blind to both, yet jobs run on whatever is on disk. A failed listing
// counts as hidden edits (fail closed).
func hiddenEdits(ctx context.Context, c Capture, root string) bool {
	out, ok := git(ctx, c, root, "ls-files", "-v", "-z")
	if !ok {
		return true
	}
	for _, rec := range strings.Split(out, "\x00") {
		if len(rec) < 3 {
			continue
		}
		tag, name := rec[0], rec[2:]
		if tag >= 'a' && tag <= 'z' { // assume-unchanged
			return true
		}
		if tag == 'S' {
			if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); err == nil {
				return true
			}
		}
	}
	return false
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
