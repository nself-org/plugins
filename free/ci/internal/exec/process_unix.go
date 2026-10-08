//go:build !windows

package exec

import (
	"os"
	osexec "os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func prepareProcess(cmd *osexec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}
func terminateGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGTERM) }
func killGroup(pid int) error      { return syscall.Kill(-pid, syscall.SIGKILL) }
func groupAlive(pid int) bool      { return syscall.Kill(-pid, 0) == nil }
func killPID(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	_ = terminateGroup(pid)
	return killGroup(pid)
}
func isTerminal() bool {
	var st syscall.Stat_t
	return syscall.Fstat(0, &st) == nil && st.Mode&syscall.S_IFMT == syscall.S_IFCHR
}

// limitedCommand installs child-only rlimits before replacing the shell with the job.
func limitedCommand(s JobSpec) []string {
	var limits string
	if s.Timeout > 0 {
		seconds := int((s.Timeout + time.Second - 1) / time.Second)
		limits += "ulimit -t " + strconv.Itoa(seconds) + " || exit 125; "
	}
	if s.MemoryMB > 0 && runtime.GOOS == "linux" {
		limits += "ulimit -v " + strconv.Itoa(s.MemoryMB*1024) + " || exit 125; "
	}
	if limits == "" {
		return s.Command
	}
	return append([]string{"sh", "-c", limits + "exec \"$@\"", "ci-job"}, s.Command...)
}
