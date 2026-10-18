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
