package tart

// Purpose: one clone per attempt: its name, its ownership marker, and its removal.
// Inputs:  the attempt id, the CI home and the Tart binary.
// Outputs: CloneName (`nself-ci-<attempt>`), a locked marker file that labels the clone for the orphan sweep,
//          and destroyClone, which leaves no clone behind or says so.
// Constraints: the marker is created and locked BEFORE the clone exists and is locked for the life of the
//          attempt, so a dead owner (kill -9 included) is exactly an unlocked marker; destroyClone retries
//          and verifies with `tart list`, because `tart delete` fails while the VM is still stopping.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClonePrefix starts the name of every clone this profile creates.
const ClonePrefix = "nself-ci-"

// CloneName is the clone of one attempt.
func CloneName(attempt string) string { return ClonePrefix + attempt }

// marker is the label of a clone: who made it.
type marker struct {
	Name        string    `json:"name"`
	Attempt     string    `json:"attempt"`
	Coordinator string    `json:"coordinator"`
	PID         int       `json:"pid"`
	Created     time.Time `json:"created"`
}

// claim is a held marker.
type claim struct {
	f    *os.File
	path string
	dir  string
	name string
}

func markerPath(dir, name string) string { return filepath.Join(dir, name+".json") }

// claimMarker creates and locks the marker for name. It fails when the marker exists and is locked
// (a live attempt of the same id) and takes over an unlocked one (its owner died).
func claimMarker(dir, name string, m marker) (*claim, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := markerPath(dir, name)
	var f *os.File
	for try := 0; ; try++ {
		var err error
		if f, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600); err != nil {
			return nil, err
		}
		ok, err := tryLock(f)
		if err != nil || !ok {
			_ = f.Close()
			if err == nil {
				err = fmt.Errorf("clone %s belongs to a live attempt", name)
			}
			return nil, err
		}
		// a sweep may have locked, deleted and closed the file between our open and our lock: the lock is then
		// on an unlinked inode and the label would not exist on disk
		if samePath(f, path) {
			break
		}
		_ = f.Close()
		if try >= 5 {
			return nil, fmt.Errorf("clone %s: the marker keeps being replaced", name)
		}
	}
	data, err := json.Marshal(m)
	if err == nil {
		if err = f.Truncate(0); err == nil {
			_, err = f.WriteAt(data, 0)
		}
	}
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &claim{f: f, path: path, dir: dir, name: name}, nil
}

// samePath reports whether the open file is the file currently at path.
func samePath(f *os.File, path string) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	pi, err := os.Stat(path)
	return err == nil && os.SameFile(fi, pi)
}

// unlockKeep drops the lock but keeps the label files, so a sweep finds the clone that could not be removed.
func (c *claim) unlockKeep() {
	if c != nil && c.f != nil {
		_ = c.f.Close()
		c.f = nil
	}
}

// release drops the label files and the lock. Call it only after the clone is gone.
func (c *claim) release() {
	if c == nil || c.f == nil {
		return
	}
	_ = os.Remove(c.path)
	_ = os.Remove(filepath.Join(c.dir, c.name+".ssh"))
	_ = os.Remove(filepath.Join(c.dir, c.name+".log"))
	_ = c.f.Close()
	c.f = nil
}

// destroyAttempts and destroyPause bound the removal loop.
const (
	destroyAttempts = 4
	destroyTimeout  = 2 * time.Minute
)

var destroyPause = time.Second

// destroyClone stops and deletes the clone and returns nil only when `tart list` no longer shows it. The
// context is detached from the caller's cancellation: a cancelled attempt must still clean up. proc, when
// set, is the VM process this run started; it is killed if `tart stop` did not end it.
func destroyClone(ctx context.Context, tool Tool, bin, name string, proc Proc) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), destroyTimeout)
	defer cancel()
	var last error
	for i := 0; i < destroyAttempts; i++ {
		vms, err := listVMs(ctx, tool, bin)
		if err != nil {
			last = err
		} else if !hasVM(vms, name) {
			return nil
		} else {
			last = fmt.Errorf("clone %s is still listed", name)
		}
		_, _ = tool.Run(ctx, []string{bin, "stop", name}, nil, nil)
		if proc != nil {
			waitExit(ctx, proc, 10*time.Second)
		}
		if res, err := tool.Run(ctx, []string{bin, "delete", name}, nil, nil); err != nil {
			last = err
		} else if res.Exit != 0 {
			last = fmt.Errorf("tart delete %s exited %d: %s", name, res.Exit, strings.TrimSpace(firstLine(res.Out)))
		}
		select {
		case <-ctx.Done():
			return errors.Join(last, ctx.Err())
		case <-time.After(destroyPause):
		}
	}
	vms, err := listVMs(ctx, tool, bin)
	if err == nil && !hasVM(vms, name) {
		return nil
	}
	return last
}

func hasVM(vms []vmInfo, name string) bool {
	for _, v := range vms {
		if v.Name == name {
			return true
		}
	}
	return false
}

// waitExit waits for the VM process to end and kills it when it does not.
func waitExit(ctx context.Context, p Proc, d time.Duration) {
	select {
	case <-p.Exited():
		return
	case <-ctx.Done():
	case <-time.After(d):
	}
	_ = p.Kill()
}
