package evidence

// Purpose: registers the two seams of the release gate (D-1074): the input
// digests (bound at the revision, now of the working tree) and the worktree
// probe. internal/evidence imports internal/run, so run cannot call it.
// Inputs:  the repository root and revision of the release attempt.
// Outputs: run.ReleaseInputs and run.ReleaseWorktree, set at package init.
// Constraints: a seam a caller already set stays (tests and embedders); the
// probe is the run's own binding (ReadWorktree), so the gate and the evidence
// agree on dirty: git status, assume-unchanged and present skip-worktree files,
// and any failed git call, are dirty; an unreadable HEAD gives sha "".

import (
	"context"
	"os"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func init() {
	if cirun.ReleaseInputs == nil {
		cirun.ReleaseInputs = releaseInputs
	}
	if cirun.ReleaseWorktree == nil {
		cirun.ReleaseWorktree = releaseWorktree
	}
}

// releaseInputs digests the inputs at rev (bound) and in the tree under root (now).
func releaseInputs(ctx context.Context, root, rev string) (bound, now model.InputDigests, err error) {
	if bound, err = InputsAt(ctx, nil, root, rev); err != nil {
		return bound, model.InputDigests{}, err
	}
	now, err = InputsOf(os.DirFS(root))
	return bound, now, err
}

// releaseWorktree is the full HEAD sha of root and whether the tree is dirty.
func releaseWorktree(ctx context.Context, root string) (string, bool) {
	sha, ok := git(ctx, nil, root, "rev-parse", "HEAD")
	if sha = strings.TrimSpace(sha); !ok || !fullRevision.MatchString(sha) {
		return "", true
	}
	return sha, ReadWorktree(ctx, nil, root).Dirty
}
