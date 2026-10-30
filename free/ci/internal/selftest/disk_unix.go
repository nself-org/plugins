//go:build !windows

package selftest

import (
	"fmt"
	"syscall"
)

// FreeMB returns the megabytes a non-root user may still write under dir.
func FreeMB(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	free := int64(st.Bavail) * int64(st.Bsize) / (1 << 20)
	if free < 0 {
		return 0, fmt.Errorf("statfs %s returned a negative size", dir)
	}
	return free, nil
}
