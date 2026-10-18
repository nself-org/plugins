//go:build !windows

package hygiene

// Purpose: find files a job UID left outside its root (temporary directories, shallow).
// Inputs:  directories to search, the job UID, the attempt root to ignore.
// Outputs: paths owned by the UID.
// Constraints: Lstat only, no symlink is followed, two levels deep, at most maxScanned entries.

import (
	"os"
	"path/filepath"
	"syscall"
)

const (
	maxScanned  = 5000
	maxScanDeep = 2
)

func ownedFiles(dirs []string, uid int, skip string) []string {
	var found []string
	scanned := 0
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if scanned >= maxScanned {
				return
			}
			scanned++
			path := filepath.Join(dir, e.Name())
			if path == skip {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil {
				continue
			}
			if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) == uid {
				found = append(found, path)
			}
			if info.IsDir() && depth < maxScanDeep {
				walk(path, depth+1)
			}
		}
	}
	for _, d := range dirs {
		walk(d, 1)
	}
	return found
}
