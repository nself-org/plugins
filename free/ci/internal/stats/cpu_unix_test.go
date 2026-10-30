//go:build unix

package stats

import (
	"syscall"
	"time"
)

// cpuTime is the user plus system CPU time this process has used, so a read measured with it is not stretched
// by other processes competing for the cores.
func cpuTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return time.Duration(time.Now().UnixNano())
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
