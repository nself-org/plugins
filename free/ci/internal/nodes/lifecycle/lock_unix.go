//go:build !windows

package lifecycle

// Purpose: one lock per node id across processes, so two `nodes add` of the same node, or a `rotate`
// racing a `revoke`, cannot undo each other's pin (review finding: a losing add removed the winner's pin).
// Inputs:  the CI home and a node id. Outputs: an unlock function.
// Constraints: flock on $NSELF_CI_HOME/.node-<id>.lock (0600); held for one command; blocking, and a
//          lock that cannot be taken is an error, never skipped.

import (
	"os"
	"path/filepath"
	"syscall"
)

func (d *Deps) lockNode(id string) (func(), error) {
	if err := os.MkdirAll(d.Home, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(d.Home, ".node-"+id+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
