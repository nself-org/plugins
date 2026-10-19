//go:build unix

package signing

// Purpose: the unix key-management lock: an exclusive flock on a per-project
// file in LockDir (the user's home, never the project, so no untracked file
// appears in the repository), so two `key init` or `key rotate` processes never overlap.
// Inputs:  CLIStore.LockDir. Outputs: an unlock func.
// Constraints: the lock file is 0600 in a 0700 directory; the lock is released
// when the func is called or the process ends; no LockDir means no lock (tests
// and callers that serialize themselves).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Lock implements SecretStore.
func (s *CLIStore) Lock(ctx context.Context) (func(), error) {
	if s.LockDir == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(s.LockDir, 0o700); err != nil {
		return nil, Failf(ReasonSignerFailed, "the key lock directory could not be created")
	}
	f, err := os.OpenFile(filepath.Join(s.LockDir, s.lockName()), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, Failf(ReasonSignerFailed, "the key lock could not be opened")
	}
	if err := flock(ctx, f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// lockName names the lock after the project root, so every process that
// replaces this project's key shares one lock whatever its CI home is.
func (s *CLIStore) lockName() string {
	sum := sha256.Sum256([]byte(filepath.Clean(s.Project)))
	return "release-key-" + hex.EncodeToString(sum[:8]) + ".lock"
}

// lockPoll is how often a waiting process retries the lock.
const lockPoll = time.Duration(5e7)

// flock takes the lock without blocking forever: it polls so ctx can end the wait.
func flock(ctx context.Context, f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != syscall.EWOULDBLOCK {
			return Failf(ReasonSignerFailed, "the key lock could not be taken")
		}
		select {
		case <-ctx.Done():
			return Failf(ReasonSignerFailed, "another key operation holds the lock")
		case <-time.After(lockPoll):
		}
	}
}
