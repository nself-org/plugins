package checkout

// Purpose: tells a live process's work directory from a dead one's, so the orphan
// sweep never removes files a sibling `serve --stdio` is still using.
// Inputs:  a directory.
// Outputs: Hold (mark in use until released) and InUse (is another process holding it).
// Constraints: the mark is an advisory lock on <dir>/.owner that the kernel drops when the
// holder exits or crashes, so there is no pid file to go stale; where advisory locks are not
// available (non-unix) nothing is ever reported in use and the sweep falls back to age alone.

import (
	"errors"
	"os"
	"path/filepath"
)

// OwnerFile is the lock file inside a directory held by Hold.
const OwnerFile = ".owner"

// Hold marks dir as in use by this process until release is called (or the process ends).
func Hold(dir string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(dir, OwnerFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if !tryLock(f) {
		_ = f.Close()
		return nil, errors.New("checkout: directory is held by another process")
	}
	return func() { _ = f.Close() }, nil
}

// InUse reports whether a live process holds dir. A directory with no owner file is not in use.
func InUse(dir string) bool {
	f, err := os.OpenFile(filepath.Join(dir, OwnerFile), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() // closing also drops a lock taken here
	return !tryLock(f)
}
