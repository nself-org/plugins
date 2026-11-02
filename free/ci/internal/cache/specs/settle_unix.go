//go:build linux || darwin

package specs

// Purpose: set a file's access and modification time through its descriptor.
// Inputs:  an open file and a time. Outputs: nil, or the syscall error.
// Constraints: the archive stores whole seconds (pack truncates), so the
// microsecond timeval loses nothing; the descriptor is used without Fd(), which
// would switch the file to blocking mode.

import (
	"os"
	"syscall"
	"time"
)

func setTimes(f *os.File, at time.Time) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	tv := []syscall.Timeval{syscall.NsecToTimeval(at.UnixNano()), syscall.NsecToTimeval(at.UnixNano())}
	var serr error
	if err := rc.Control(func(fd uintptr) { serr = syscall.Futimes(int(fd), tv) }); err != nil {
		return err
	}
	return serr
}
