//go:build !windows

package exec

import (
	osexec "os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func prepareProcess(cmd *osexec.Cmd) error {
	if err := ensureSubreaper(); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}
func terminateGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGTERM) }
func killGroup(pid int) error      { return syscall.Kill(-pid, syscall.SIGKILL) }
func isTerminal() bool {
	var st syscall.Stat_t
	return syscall.Fstat(0, &st) == nil && st.Mode&syscall.S_IFMT == syscall.S_IFCHR
}

// limitedCommand installs child-only rlimits before replacing the shell with the job.
func limitedCommand(s JobSpec) []string { return limitedCommandOn(s, runtime.GOOS) }

// limitedCommandOn is limitedCommand for a named OS. The memory cap is RLIMIT_DATA (ulimit -d),
// which counts writable private memory; RLIMIT_AS (ulimit -v) counts reserved address space and
// kills the Go and V8 runtimes at start, so it is never used.
func limitedCommandOn(s JobSpec, goos string) []string {
	var limits string
	if s.Timeout > 0 {
		seconds := int((s.Timeout + time.Second - 1) / time.Second)
		limits += "ulimit -t " + strconv.Itoa(seconds) + " || exit 125; "
	}
	if s.MemoryLimitMB > 0 && goos == "linux" {
		limits += "ulimit -d " + strconv.Itoa(s.MemoryLimitMB*1024) + " || exit 125; "
	}
	if limits == "" {
		return s.Command
	}
	return append([]string{"sh", "-c", limits + "exec \"$@\"", "ci-job"}, s.Command...)
}
