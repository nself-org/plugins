package checkout

// Purpose: the agent half, part 2: turn a verified bundle into a detached
// checkout of the lease's exact revision inside the job root.
// Inputs:  the lease and the job root directory.
// Outputs: the source directory (<root>/src).
// Constraints: Stage waits for the digest-verified bundle and refuses before
// any git process runs when verification failed; git fetches only the private
// refs/nself-ci/ namespace and the resulting HEAD must equal the lease's
// revision; the bundle is deleted from staging once it is unpacked.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/protocol"
)

// Stage waits for the lease's bundle, then unpacks it into <jobRoot>/src and
// returns that directory. The staging root of the lease is removed on every exit.
func (r *Receiver) Stage(ctx context.Context, l protocol.Lease, jobRoot string) (string, error) {
	if l.Checkout.Size <= 0 {
		return "", nil
	}
	r.mu.Lock()
	t := r.open[key{l.LeaseID, l.Epoch}]
	if t != nil {
		t.staging = true
	}
	r.mu.Unlock()
	if t == nil {
		return "", fmt.Errorf("checkout: no transfer announced for lease %s", l.LeaseID)
	}
	defer r.Forget(l.LeaseID, l.Epoch)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-t.done:
	}
	r.mu.Lock()
	err := t.err
	r.mu.Unlock()
	if err != nil {
		return "", err
	}
	src := filepath.Join(jobRoot, "src")
	if err := os.Mkdir(src, 0o700); err != nil {
		return "", err
	}
	steps := [][]string{
		{"-C", src, "init", "--quiet"},
		{"-C", src, "bundle", "verify", t.path},
		{"-C", src, "fetch", "--quiet", "--no-tags", "--no-recurse-submodules", t.path, "+" + RefPrefix + "*:" + RefPrefix + "*"},
		{"-C", src, "checkout", "--quiet", "--detach", "--force", l.Checkout.Revision},
	}
	for _, args := range steps {
		if _, err := git(ctx, r.run, args...); err != nil {
			return "", fmt.Errorf("checkout: unpack bundle: %w", err)
		}
	}
	head, err := git(ctx, r.run, "-C", src, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if head != l.Checkout.Revision {
		return "", fmt.Errorf("checkout: bundle HEAD %s is not the leased revision", head)
	}
	return src, nil
}
