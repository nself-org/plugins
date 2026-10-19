package readiness

// Purpose: the per-host run lock, so two watchers on one machine never double-run.
// Inputs: the lock file path (conventionally $NSELF_CI_HOME/readiness.lock).
// Outputs: a held Lock, or E799 naming the holder's pid when another process holds it.
// Constraints: an advisory flock on the open file, so the kernel drops it when the holder
// dies and a stale file from a dead pid is simply taken over (the pid inside is only
// for the message). The file is never deleted: removing it would let a second process
// lock a different inode.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Lock is a held run lock.
type Lock struct{ f *os.File }

// AcquireLock takes the lock at path without waiting.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// The file may be unlinked (a cleanup job, an operator) between the open and the flock.
	// A lock on an unlinked inode excludes nobody, so check the path still names the inode
	// we hold and retry on the new file otherwise.
	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
		if err != nil {
			return nil, err
		}
		if err := flock(f); err != nil {
			holder := readHolder(f)
			_ = f.Close()
			if isBusy(err) {
				return nil, coded("E799", "another watcher holds the run lock"+holder)
			}
			return nil, err
		}
		if !stillAtPath(f, path) {
			_ = f.Close()
			continue
		}
		if err := f.Truncate(0); err == nil {
			_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
		}
		return &Lock{f: f}, nil
	}
	return nil, fmt.Errorf("run lock file %s keeps being replaced", path)
}

// stillAtPath reports whether path still names the open file.
func stillAtPath(f *os.File, path string) bool {
	held, err1 := f.Stat()
	named, err2 := os.Stat(path)
	return err1 == nil && err2 == nil && os.SameFile(held, named)
}

// Release drops the lock. Calling it twice is safe.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close() // closing the descriptor releases the flock
	l.f = nil
	return err
}

func readHolder(f *os.File) string {
	b := make([]byte, 32)
	n, _ := f.ReadAt(b, 0)
	pid := strings.TrimSpace(string(b[:n]))
	if _, err := strconv.Atoi(pid); err != nil {
		return ""
	}
	return fmt.Sprintf(" (pid %s)", pid)
}
