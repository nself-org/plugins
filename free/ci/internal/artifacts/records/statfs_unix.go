//go:build linux || darwin || freebsd

package records

import "syscall"

// volumeSpace reports the free bytes (available to this user) and the total
// bytes of the volume holding path.
func volumeSpace(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), uint64(st.Blocks) * uint64(st.Bsize), nil
}
