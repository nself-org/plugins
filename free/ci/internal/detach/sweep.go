package detach

// Purpose: remove the event files of runs that no longer exist (P7-CI-63, F12):
// `ci runs prune` and the run-start retention delete a run's row, and its
// detached stream goes with it.
// Inputs:  the state directory and a "does this run still exist" answer.
// Outputs: the removed file names.
// Constraints: a file younger than OrphanGrace is never removed (its run row
// may not be written yet); a file naming a known run is never removed; a dry
// run removes nothing.

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OrphanGrace is how old a stream file must be before it can be swept.
const OrphanGrace = time.Hour

// Sweep deletes detached stream files whose run is gone. known reports whether
// the run still has a row; a file with no run id at all (the child died before
// it queued) counts as gone. now zero means time.Now().
func Sweep(home string, known func(runID string) bool, now time.Time, dryRun bool) ([]string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	entries, err := os.ReadDir(Dir(home))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ndjson") {
			continue
		}
		path := filepath.Join(Dir(home), e.Name())
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < OrphanGrace {
			continue
		}
		if id := RunIDOf(path); id != "" && known(id) {
			continue
		}
		if !dryRun {
			if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
				return removed, err
			}
		}
		removed = append(removed, e.Name())
	}
	return removed, nil
}

// Remove deletes the stream files of the named runs (their rows were pruned).
func Remove(home string, runIDs []string) ([]string, error) {
	var removed []string
	for _, id := range runIDs {
		path := StreamPath(home, id)
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		removed = append(removed, filepath.Base(path))
	}
	return removed, nil
}
