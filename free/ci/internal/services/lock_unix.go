//go:build !windows

package services

// Purpose: an advisory lock that proves a provision's owner is alive. The
// owner holds an exclusive flock on the provision's lock file for the whole
// life of the provision; the kernel drops it when the owner dies, kill -9
// included, and a recycled process id cannot fake it.
// Inputs:  a lock file path. Outputs: a held lock, or "owner is alive".
// Constraints: Unlock is idempotent.

import (
	"os"
	"syscall"
)

// ownerLock is a held flock.
type ownerLock struct{ f *os.File }

// acquireLock takes the lock on path, creating the file. held is true when
// another holder has it.
func acquireLock(path string) (l *ownerLock, held bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, true, nil
		}
		return nil, false, err
	}
	return &ownerLock{f: f}, false, nil
}

// Unlock releases the lock.
func (l *ownerLock) Unlock() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
	l.f = nil
}

// active reports whether the lock is currently held by this value.
func (l *ownerLock) active() bool { return l != nil && l.f != nil }
