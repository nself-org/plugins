//go:build !windows

package main

// Purpose: the Unix half of the job-shim ops (D-1230): kill every process of the calling job user and
// check that a directory belongs to it.
// Inputs:  none (the calling process's own UID); a FileInfo from Lstat.
// Outputs: nil when no process of the user is left to signal; whether the entry's owner is the euid.
// Constraints: kill(-1) signals only processes the caller may signal (its own UID) and never the caller
//          itself; it is refused outright as root.

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

func killOwnProcesses() error {
	if os.Geteuid() <= 0 {
		return errors.New("refusing to signal every process as root")
	}
	if err := syscall.Kill(-1, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func ownedBySelf(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}
