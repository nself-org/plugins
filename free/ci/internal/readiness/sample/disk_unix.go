//go:build unix

package sample

import "syscall"

// statfsFree returns the bytes a normal user can still write under path.
func statfsFree(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:unconvert // field widths differ by platform
}
