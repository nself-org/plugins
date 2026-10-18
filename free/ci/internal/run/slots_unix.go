//go:build !windows

package run

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lockSlot takes slot n's advisory lock. ok is false when another process holds it; when the
// lock file cannot be used at all the slot is granted process-locally (nil file).
func lockSlot(dir string, n int) (*os.File, bool) {
	if dir == "" || os.MkdirAll(dir, 0700) != nil {
		return nil, true
	}
	f, err := os.OpenFile(filepath.Join(dir, "slot-"+itoa(n)+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, !errors.Is(err, syscall.EWOULDBLOCK)
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
