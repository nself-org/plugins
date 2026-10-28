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
	"strings"
	"syscall"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

const lockPoll = 50 * time.Millisecond

func lockRun(ctx context.Context, home, runID string) (func(), error) {
	if err := os.MkdirAll(releasesDir(home), 0o700); err != nil {
		return nil, refuse(signing.ReasonSignerFailed, "the releases directory could not be created")
	}
	sweepLocks(home)
	path := filepath.Join(releasesDir(home), ".lock-"+runID)
	var f *os.File
	for {
		var err error
		f, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
		if err != nil {
			return nil, refuse(signing.ReasonSignerFailed, "the release lock could not be opened")
		}
		st, err := f.Stat()
		if err != nil || !st.Mode().IsRegular() {
			_ = f.Close()
			return nil, refuse(signing.ReasonSignerFailed, "the release lock is not a regular file")
		}
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// A sweeper may have unlinked this inode while we waited. Never
			// publish under a detached lock: retry against the current name.
			at, serr := os.Lstat(path)
			if serr == nil && os.SameFile(st, at) {
				break
			}
			_ = f.Close()
			continue
		}
		_ = f.Close()
		if err != syscall.EWOULDBLOCK {
			return nil, refuse(signing.ReasonSignerFailed, "the release lock could not be taken")
		}
		select {
		case <-ctx.Done():
			return nil, refuse(signing.ReasonSignerFailed, "another publication of this run holds the lock")
		case <-time.After(lockPoll):
		}
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// sweepLocks removes only aged, idle lock inodes. lockRun checks the name
// again after claiming, so contenders never publish under an unlinked inode.
func sweepLocks(home string) {
	entries, err := os.ReadDir(releasesDir(home))
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".lock-") {
			continue
		}
		path := filepath.Join(releasesDir(home), e.Name())
		f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			continue
		}
		func() {
			defer f.Close()
			st, err := f.Stat()
			if err != nil || !st.Mode().IsRegular() || time.Since(st.ModTime()) <= staleAfter {
				return
			}
			if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
				return
			}
			defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			if at, err := os.Lstat(path); err == nil && os.SameFile(st, at) {
				_ = os.Remove(path)
			}
		}()
	}
}
