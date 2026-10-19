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
	"strings"
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
	f, err := os.OpenFile(LockPath(home), os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
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
	f, err := os.OpenFile(LockPath(home), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true // held exclusively by someone else
		}
		c, derr := net.DialTimeout("unix", SocketFor(home), time.Second) // locks unsupported here (a network mount): ask the socket
		if derr == nil {
			_ = c.Close()
		}
		return derr == nil
	}
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

// RecordSocket notes in the lock file where the serve's socket is, so a client finds it even when the
// serve was started with another --api-socket path.
func RecordSocket(home, path string) error {
	raw, _ := os.ReadFile(LockPath(home))
	var keep []string
	for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if l != "" && !strings.HasPrefix(l, "socket=") {
			keep = append(keep, l)
		}
	}
	return os.WriteFile(LockPath(home), []byte(strings.Join(append(keep, "socket="+path), "\n")+"\n"), 0o600)
}

// SocketFor is the socket of the serve that holds home: the path it recorded, else the default.
func SocketFor(home string) string {
	raw, err := os.ReadFile(LockPath(home))
	if err == nil {
		for _, l := range strings.Split(string(raw), "\n") {
			if p, ok := strings.CutPrefix(l, "socket="); ok && filepath.IsAbs(p) {
				return p
			}
		}
	}
	return SocketPath(home)
}

// SocketIsOurs reports whether path is a unix socket owned by this user (not a symlink, not another
// user's): a client never sends a run to a socket someone else could have planted.
func SocketIsOurs(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
