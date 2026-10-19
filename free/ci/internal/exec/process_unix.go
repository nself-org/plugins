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
	if err := ensureSubreaper(); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}
func terminateGroup(pid int) error { return syscall.Kill(-pid, syscall.SIGTERM) }
func killGroup(pid int) error      { return syscall.Kill(-pid, syscall.SIGKILL) }

// killMarkedDescendants kills the recorded descendants whose start identity still matches, so a
// job process that escaped the leader's group is not left running. A pid whose identity cannot be
// read or no longer matches is skipped: it is gone or belongs to someone else.
func killMarkedDescendants(procs []markerProc) {
	for _, p := range procs {
		if p.PID <= 1 || p.PID == os.Getpid() {
			continue
		}
		if same, known := sameProcess(p.PID, p.Identity); known && same {
			_ = syscall.Kill(p.PID, syscall.SIGKILL)
		}
	}
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
