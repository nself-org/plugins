//go:build !windows

package lifecycle

// Purpose: file locks across CLI processes. One lock per node id orders the commands of a node (a rotate
// cannot race a revoke); one home-wide lock orders every read-modify-write of the shared pin file (the
// pin file is rewritten whole, so two processes working on different nodes would otherwise lose each
// other's change, which can undo a rotation or bring a revoked pin back).
// Inputs:  the CI home and a node id. Outputs: an unlock function.
// Constraints: flock on $NSELF_CI_HOME/.node-<id>.lock and .known_hosts.lock (0600); the wait is bounded
//          (lockWait) and a busy lock is E656 "busy", never an indefinite hang; the node id must already
//          be a validated node id (Run validates it). The lock files are empty, 0600 and kept on purpose,
//          also after `nodes remove`: unlinking a flock file while another process waits on it lets a third
//          process lock a new file at the same path, so two holders would both think they own the node.
//          They cost one inode per node id ever used and may be deleted by hand when no `nodes` command runs.

import (
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockWait bounds how long a command waits for a lock held by another process.
const lockWait = 30 * time.Second

func (d *Deps) lockNode(id string) (func(), error) { return d.lockFile(".node-" + id + ".lock") }

// lockPins serializes every change of the shared pin file.
func (d *Deps) lockPins() (func(), error) { return d.lockFile(".known_hosts.lock") }

func (d *Deps) lockFile(name string) (func(), error) {
	if err := os.MkdirAll(d.Home, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(d.Home, name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for end := time.Now().Add(lockWait); ; time.Sleep(50 * time.Millisecond) {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK || time.Now().After(end) {
			_ = f.Close()
			return nil, err
		}
	}
}
