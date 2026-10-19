//go:build unix

package entries

import "syscall"

// statfs reports free (available to a normal user) and total bytes of the volume holding path.
func statfs(path string) (free, total int64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), int64(st.Blocks) * int64(st.Bsize), nil
}
