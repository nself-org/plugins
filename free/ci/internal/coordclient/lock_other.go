//go:build !unix

package coordclient

import (
	"errors"
	"path/filepath"
)

// SocketPath is the unix socket of a local serve.
func SocketPath(home string) string { return filepath.Join(home, "serve.sock") }

// LockPath is the lock file of a local serve.
func LockPath(home string) string { return filepath.Join(home, "serve.lock") }

// AcquireServeLock is unavailable here: a local serve is not supported on this platform.
func AcquireServeLock(string) (func(), error) {
	return nil, errors.New("a local ci serve lock is not supported on this platform")
}

// ServeHoldsLock is always false here.
func ServeHoldsLock(string) bool { return false }

// RecordSocket is a no-op here.
func RecordSocket(string, string) error { return nil }

// SocketFor is the default socket path here.
func SocketFor(home string) string { return SocketPath(home) }

// SocketIsOurs is always true here (there is no local serve).
func SocketIsOurs(string) bool { return true }
