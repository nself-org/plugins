package slots

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// startTime is the process start time from the kernel's process table.
func startTime(pid int) string {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return ""
	}
	tv := kp.Proc.P_starttime
	return strconv.FormatInt(int64(tv.Sec), 10) + "." + strconv.FormatInt(int64(tv.Usec), 10)
}
