package policy

// Purpose: choose the commit the protected-branch scope is read at. A stored
// admin pin wins when it is reachable from the fetched default branch; an
// unreachable pin is ignored (E672) and the fetched tip is used.
// Inputs: the stored pin, the Branch and a Runner.
// Outputs: the commit to read and the Ignored note, if any.
// Constraints: reachability is `git merge-base --is-ancestor <pin> <tip>`
// through the Runner; an unknown or malformed pin counts as unreachable.

import (
	"context"
	"errors"
	"os"
)

func isNotExist(err error) bool { return errors.Is(err, os.ErrNotExist) }

// chooseCommit returns the commit to read the protected branch at.
func chooseCommit(ctx context.Context, run Runner, root string, b Branch, pin string) (string, []Ignored) {
	if pin == "" || b.Tip == "" {
		return b.Tip, nil
	}
	if pin == b.Tip || reachable(ctx, run, root, pin, "refs/remotes/origin/"+b.Name) {
		return pin, nil
	}
	return b.Tip, []Ignored{{Key: "pinned_commit", Scope: SourceProtectedBranch, Reason: "E672 stored pin is not reachable from origin/" + b.Name + "; the fetched tip is used"}}
}

func reachable(ctx context.Context, run Runner, root, sha, ref string) bool {
	if !fullSHA.MatchString(sha) {
		return false
	}
	g := gitRunner{run: run, root: root}
	_, ok := g.git(ctx, "merge-base", "--is-ancestor", sha, ref)
	return ok
}
