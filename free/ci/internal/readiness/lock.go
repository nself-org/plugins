package readiness

// Purpose: the per-host run lock, so two watchers on one machine never double-run.
// Inputs: the lock file path (conventionally $NSELF_CI_HOME/readiness.lock).
// Outputs: a held Lock, or E799 naming the holder's pid when another process holds it.
// Constraints: an advisory flock on the open file, so the kernel drops it when the holder
// dies and a stale file from a dead pid is simply taken over (the pid inside is only
// for the message). This code never deletes the file: removing it would let a second
// process lock a different inode; Held() lets the owner notice when something else did.
// The holder keeps the *Lock until Release (the package pins it against garbage collection).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Lock is a held run lock.
type Lock struct {
	f    *os.File
	path string
}

// pinned keeps every held Lock reachable: an *os.File closes itself when it is garbage
// collected, which would silently release the flock of a caller that dropped the *Lock.
var pinned = struct {
	sync.Mutex
	locks map[*Lock]struct{}
}{locks: map[*Lock]struct{}{}}

func pin(l *Lock) {
	pinned.Lock()
	defer pinned.Unlock()
	pinned.locks[l] = struct{}{}
}

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
		l := &Lock{f: f, path: path}
		pin(l)
		return l, nil
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
	pinned.Lock()
	delete(pinned.locks, l)
	pinned.Unlock()
	err := l.f.Close() // closing the descriptor releases the flock
	l.f = nil
	return err
}

// Held reports whether this process still holds the lock on the file now at the path. It is
// false after Release and after the file was deleted or replaced (a second watcher could then
// lock the new file), so a watcher checks it before each claim and stops when it turns false.
func (l *Lock) Held() bool {
	return l != nil && l.f != nil && stillAtPath(l.f, l.path)
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
