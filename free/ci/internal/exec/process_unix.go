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

// limitedCommandOn is the job's command behind a shell that sets child-only rlimits first, or the bare
// command when no limit applies. The memory cap is RLIMIT_DATA (ulimit -d), which counts writable private
// memory; RLIMIT_AS (ulimit -v) counts reserved address space and kills the Go and V8 runtimes at start,
// so it is never used.
func limitedCommandOn(s JobSpec, goos string) []string {
	limits := limitScript(s, goos)
	if limits == "" {
		return s.Command
	}
	return append([]string{"sh", "-c", limits + "exec \"$@\"", "ci-job"}, s.Command...)
}

// limitScript is the ulimit lines for the job.
func limitScript(s JobSpec, goos string) string {
	var limits string
	if s.Timeout > 0 {
		seconds := int((s.Timeout + time.Second - 1) / time.Second)
		limits += "ulimit -t " + strconv.Itoa(seconds) + " || exit 125; "
	}
	if s.MemoryLimitMB > 0 && goos == "linux" {
		limits += "ulimit -d " + strconv.Itoa(s.MemoryLimitMB*1024) + " || exit 125; "
	}
	return limits
}

// gatedCommand is the job's argv with the start gate (process_gate.go): the shell sets the rlimits first, so
// they hold from the payload's first instruction, then waits for one line on fd 3 and only then closes fd 3
// and replaces itself with the payload. No line (end of file) means the agent never recorded the job: exit
// 125 without running it. It always wraps the command, so there is one sh start per job; the pid is the
// payload's too (exec keeps it), which is what the marker records.
func gatedCommand(s JobSpec) ([]string, bool) {
	script := limitScript(s, runtime.GOOS) + "read -r g <&3 || exit 125; exec 3<&-; exec \"$@\""
	return append([]string{"sh", "-c", script, "ci-job"}, s.Command...), true
}

var signalNames = map[syscall.Signal]string{
	syscall.SIGHUP: "SIGHUP", syscall.SIGINT: "SIGINT", syscall.SIGQUIT: "SIGQUIT", syscall.SIGILL: "SIGILL",
	syscall.SIGTRAP: "SIGTRAP", syscall.SIGABRT: "SIGABRT", syscall.SIGBUS: "SIGBUS", syscall.SIGFPE: "SIGFPE",
	syscall.SIGKILL: "SIGKILL", syscall.SIGSEGV: "SIGSEGV", syscall.SIGPIPE: "SIGPIPE", syscall.SIGALRM: "SIGALRM",
	syscall.SIGTERM: "SIGTERM", syscall.SIGSYS: "SIGSYS",
}

// exitSignal names the signal that ended the process, or "" when it exited normally.
func exitSignal(err *osexec.ExitError) string {
	ws, ok := err.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return ""
	}
	if n, ok := signalNames[ws.Signal()]; ok {
		return n
	}
	return "signal " + strconv.Itoa(int(ws.Signal()))
}
