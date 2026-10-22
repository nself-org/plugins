//go:build windows

package backup

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// LockDataDir excludes serve and restore until release, using the Windows file lock.
func LockDataDir(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "coordinator.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	lock := &windows.Overlapped{}
	if err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, lock); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("coordinator data dir is locked: %w", err)
	}
	return func() { _ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, lock); _ = f.Close() }, nil
}
