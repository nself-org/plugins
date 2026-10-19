//go:build unix

package coordclient

// Purpose: the lock and socket names of a local `ci serve` (P7-NODE-19, Epic D14, review F-lock): while a
// serve runs it holds an exclusive flock on $NSELF_CI_HOME/serve.lock, so a client learns "a serve owns
// this home" without reading its store, and the kernel drops the lock when the serve dies (a stale file
// is never mistaken for a live serve, and a reused pid cannot fake one).
// Inputs:  the CI home.
// Outputs: AcquireServeLock (serve side), ServeHoldsLock (client side), SocketPath.
// Constraints: the lock file holds `pid` and `started` for people only; ServeHoldsLock never blocks and
//          never writes; the lock file is 0600 in a 0700-created directory.

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// SocketPath is the unix socket of a local serve.
func SocketPath(home string) string { return filepath.Join(home, "serve.sock") }

// LockPath is the lock file of a local serve.
func LockPath(home string) string { return filepath.Join(home, "serve.lock") }

// AcquireServeLock takes the serve lock or fails when another serve holds it. release gives it up.
func AcquireServeLock(home string) (release func(), err error) {
	if err = os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(LockPath(home), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errors.New("another ci serve already runs on " + home)
		}
		return nil, err
	}
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "pid=%d\nstarted=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

// ServeHoldsLock reports whether a live process holds the serve lock of home.
func ServeHoldsLock(home string) bool {
	f, err := os.Open(LockPath(home))
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true // held exclusively by someone else
		}
		c, derr := net.DialTimeout("unix", SocketPath(home), time.Second) // locks unsupported here (a network mount): ask the socket
		if derr == nil {
			_ = c.Close()
		}
		return derr == nil
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}
