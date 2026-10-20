//go:build !windows

package backup

// Purpose: the data-dir lock (D-1308): `ci serve` holds it for its whole run and `serve restore` holds it for the
//          swap, so neither can start on a dir the other is using. The heartbeat check in Restore stays: the
//          lock closes the window between that check and the swap, and a first start that wrote no heartbeat yet.
// Inputs:  the data dir. Outputs: a release func, or E668 running_coordinator when another process holds it.
// Constraints: flock on <dir>/.lock (released by the kernel when the process dies, so a crash leaves no
//          stale lock); the file holds nothing; the lock is advisory and covers cooperating `nself ci` commands.

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lockName is the lock file in the data dir.
const lockName = ".lock"

// Lock takes the exclusive lock of dataDir, creating the dir when it is missing.
func Lock(dataDir string) (release func(), err error) {
	if err = os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fail(ReasonIO, "the data dir cannot be created: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(dataDir, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fail(ReasonIO, "the data dir lock cannot be opened: %v", err)
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fail(ReasonRunningCoordinator, "another `nself ci serve` or `serve restore` holds the lock on %s", dataDir)
		}
		return nil, fail(ReasonIO, "the data dir cannot be locked: %v", err)
	}
	return func() { _ = f.Close() }, nil // closing the file drops the flock
}
