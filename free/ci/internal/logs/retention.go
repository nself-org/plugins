package logs

// Purpose: the file half of log retention: list the stored runs with their
// size and age, and remove one. Which runs go is decided by internal/history,
// which also owns the store rows.
// Inputs:  the state directory. Outputs: RunInfo values, oldest first.
// Constraints: only directories whose names are valid ids are touched; a
// symlink is never followed or removed as a directory; RemoveRun removes the
// whole run directory, so the log files and their directory leave together.

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunInfo is one stored run's log directory.
type RunInfo struct {
	Run      string
	Bytes    int64     // size on disk (segments are compressed)
	Modified time.Time // newest change inside
}

// ListRuns returns the runs that have a log directory, oldest first. No logs
// directory yet is an empty list.
func ListRuns(home string) ([]RunInfo, error) {
	root := Root(home)
	ents, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fail(CodeStore, "cannot list the logs directory: %v", err)
	}
	var out []RunInfo
	for _, e := range ents {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		ri := RunInfo{Run: e.Name()}
		_ = filepath.WalkDir(filepath.Join(root, e.Name()), func(_ string, d fs.DirEntry, werr error) error {
			if werr != nil {
				return nil
			}
			if info, ierr := d.Info(); ierr == nil {
				if d.Type().IsRegular() {
					ri.Bytes += info.Size()
				}
				if info.ModTime().After(ri.Modified) {
					ri.Modified = info.ModTime()
				}
			}
			return nil
		})
		out = append(out, ri)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Modified.Equal(out[j].Modified) {
			return out[i].Modified.Before(out[j].Modified)
		}
		return out[i].Run < out[j].Run
	})
	return out, nil
}

// RemoveRun deletes one run's stored logs. A missing directory is not an error.
func RemoveRun(home, run string) error {
	if !ValidID(run) {
		return fail(CodeStore, "refusing to remove logs for an invalid run id")
	}
	if fi, err := os.Lstat(Root(home)); err == nil && !fi.IsDir() { // a symlinked logs directory is never followed
		return fail(CodeStore, "refusing to remove from a logs directory that is not a plain directory")
	}
	dir := RunDir(home, run)
	if fi, err := os.Lstat(dir); err == nil && !fi.IsDir() {
		return fail(CodeStore, "refusing to remove %q: not a directory", run)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fail(CodeStore, "cannot remove the logs of %s: %v", run, err)
	}
	return nil
}
