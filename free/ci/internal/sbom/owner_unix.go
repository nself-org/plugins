//go:build unix

package sbom

// Purpose: the file-ownership half of the installed-tool check on unix.
// Inputs:  an os.FileInfo from Lstat.
// Outputs: whether the current user owns the file.
// Constraints: a file whose owner cannot be read is not trusted.

import (
	"os"
	"syscall"
)

func ownedByUs(st os.FileInfo) bool {
	sys, ok := st.Sys().(*syscall.Stat_t)
	return ok && int(sys.Uid) == os.Geteuid()
}
