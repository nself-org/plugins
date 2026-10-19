//go:build unix

package publish

// Purpose: the per-run publication lock: an exclusive flock on
// <home>/releases/.lock-<run>, held from the ledger check through the
// directory install and the ledger write, so two processes (the AfterRun hook
// and `artifacts sign`, or two signs) never interleave on one run.
// Constraints: the lock file is 0600 in the 0700 releases directory; the wait
// ends with the context; the lock is released by the returned func or when the
// process ends.

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

const lockPoll = 50 * time.Millisecond

func lockRun(ctx context.Context, home, runID string) (func(), error) {
	if err := os.MkdirAll(releasesDir(home), 0o700); err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "the releases directory could not be created")
	}
	f, err := os.OpenFile(filepath.Join(releasesDir(home), ".lock-"+runID), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "the release lock could not be opened")
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK {
			_ = f.Close()
			return nil, refuse(signing.ReasonSignerFailed, "the release lock could not be taken")
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, refuse(signing.ReasonSignerFailed, "another publication of this run holds the lock")
		case <-time.After(lockPoll):
		}
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
