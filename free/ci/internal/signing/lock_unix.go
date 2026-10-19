//go:build unix

package signing

// Purpose: the unix key-management lock: an exclusive flock on a file in the
// CI home, so two `key init` or `key rotate` processes never overlap.
// Inputs:  CLIStore.LockDir. Outputs: an unlock func.
// Constraints: the lock file is 0600 in a 0700 directory; the lock is released
// when the func is called or the process ends; no LockDir means no lock (tests
// and callers that serialize themselves).

import (
	"context"
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
	f, err := os.OpenFile(filepath.Join(s.LockDir, "release-key.lock"), os.O_CREATE|os.O_RDWR, 0o600)
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
		case <-time.After(50 * time.Millisecond):
		}
	}
}
