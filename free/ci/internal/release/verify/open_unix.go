//go:build unix

package verify

// Purpose: the unix half of the release file open (D-1198).
// Constraints: the open itself refuses a symbolic link in the last part (O_NOFOLLOW) and never blocks
// on a FIFO (O_NONBLOCK), so a local writer who swaps a checked file between the check and the open
// gains nothing; the caller still checks the opened descriptor with fstat.

import (
	"os"
	"syscall"
)

// openRegular opens path read-only. follow allows a symbolic link in the last part.
func openRegular(path string, follow bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NONBLOCK | syscall.O_CLOEXEC
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	return os.OpenFile(path, flags, 0)
}
