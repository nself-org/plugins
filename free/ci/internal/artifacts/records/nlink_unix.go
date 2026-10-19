//go:build unix

package records

import (
	"io/fs"
	"syscall"
)

// linkCount is the number of names of the file's inode (1 when unknown).
func linkCount(info fs.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink)
	}
	return 1
}
