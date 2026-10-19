//go:build !windows

package hygiene

import (
	"io/fs"
	"os"
	"syscall"
)

// ownedByCaller reports whether the file belongs to the calling user.
func ownedByCaller(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid()
}

// ownerUID returns the owner of a file when it is not the caller, else 0.
func ownerUID(info fs.FileInfo) int {
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() {
		return int(st.Uid)
	}
	return 0
}
