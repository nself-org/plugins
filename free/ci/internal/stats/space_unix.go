//go:build linux || darwin || freebsd

package stats

// Purpose: free bytes of the volume that holds the run store.
// Inputs:  a path on that volume.
// Outputs: bytes available to this user.
// Constraints: ok is false when the volume cannot be read.

import "syscall"

func freeBytes(path string) (int64, bool) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, false
	}
	return int64(st.Bavail) * int64(st.Bsize), true
}
