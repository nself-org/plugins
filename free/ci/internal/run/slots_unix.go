//go:build !windows

package run

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockSlot takes slot n's advisory lock. ok is false when another process holds it or the
// lock cannot be taken (fail closed: an unusable lock file never grants a slot, or two
// coordinators could share a job UID). Only an unknown home (dir "") is process-local.
func lockSlot(dir string, n int) (*os.File, bool) {
	if dir == "" {
		return nil, true
	}
	if os.MkdirAll(dir, 0700) != nil {
		return nil, false
	}
	f, err := os.OpenFile(filepath.Join(dir, "slot-"+itoa(n)+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return f, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for ; n > 0; n /= 10 {
		d = append([]byte{byte('0' + n%10)}, d...)
	}
	return string(d)
}
