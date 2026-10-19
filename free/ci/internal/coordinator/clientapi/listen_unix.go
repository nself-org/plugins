//go:build unix

package clientapi

// Purpose: the local unix socket of a running `ci serve` (P7-NODE-19, Epic D14): $NSELF_CI_HOME/serve.sock,
// mode 0600, which a PERSONAL `nself ci` uses when the serve holds the lock.
// Inputs:  the socket path. Outputs: a net.Listener the caller serves with a Local Server.
// Constraints: the parent directory is created 0700; the socket is bound under umask 077 so it is never
//          readable by others, even for a moment; a socket file left by a dead serve is removed only when
//          nothing answers on it.

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// ListenUnix binds the socket at path.
func ListenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if c, err := net.Dial("unix", path); err == nil {
		_ = c.Close()
		return nil, errors.New("another process answers on " + path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}
