package policy

// Purpose: the object reads of the protected branch (tip commit, the config's tree entry and its bytes)
// answered by the run's batch session when it has one, else by git through the Runner (P7-CI-46).
// Inputs:  the gitRunner and the run's context (carrying the gitbatch Set).
// Outputs: the same strings and booleans the git commands gave, so the callers do not change.
// Constraints: a batch answer that is not usable fails the read exactly as a failing git would; only
//          gitbatch.ErrUnavailable (no session, git too old, object too large) runs the git command.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/gitbatch"
)

// verifyCommit is `rev-parse --verify --quiet <name>`: the id name resolves to, ok false when it does not.
func (g gitRunner) verifyCommit(ctx context.Context, name string) (string, bool) {
	if oid, ok, used := gitbatch.RevParse(ctx, g.root, name); used {
		return oid, ok
	}
	return g.git(ctx, "rev-parse", "--verify", "--quiet", name)
}

// lsTree is `ls-tree <sha> -- <rel>`: "" when the path is absent, else the "<mode> <type> <id>\t<rel>" line.
func (g gitRunner) lsTree(ctx context.Context, sha, rel string) (string, bool) {
	if e, found, ok, used := gitbatch.LsTree(ctx, g.root, sha, rel); used {
		if !ok || !found {
			return "", ok
		}
		return fmt.Sprintf("%s %s %s\t%s", e.Mode, e.Type, e.OID, rel), true
	}
	return g.git(ctx, "ls-tree", sha, "--", rel)
}

// showBlob is `show --no-textconv <sha>:./<rel>` for the blob a lsTree line named.
func (g gitRunner) showBlob(ctx context.Context, sha, rel, listed string) (string, bool) {
	if f := strings.Fields(listed); len(f) >= 3 {
		if data, ok, used := gitbatch.ReadBlob(ctx, g.root, f[2]); used {
			return string(data), ok
		}
	}
	return g.raw(ctx, "show", "--no-textconv", sha+":./"+rel)
}
