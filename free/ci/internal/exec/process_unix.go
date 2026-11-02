//go:build !windows

package exec

import (
	osexec "os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
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

// shellAdded are the variables a shell may add to the environment of what it execs (PWD, OLDPWD, SHLVL, and
// bash's _). The gated wrapper drops them, then puts back any that the job environment itself set, so the
// payload's environment is exactly cmd.Env.
var shellAdded = []string{"PWD", "OLDPWD", "SHLVL", "_"}

// gatedCommand is the job's argv with the start gate (process_gate.go): the shell sets the rlimits first, so
// they hold from the payload's first instruction, then waits for one line on fd 3 and only then closes fd 3,
// restores the environment and replaces itself with the payload. No line (end of file) means the agent never
// recorded the job: exit 125 without running it. The payload is resolved here, so a missing binary is a start
// error; its pid is the shell's (exec keeps it), which is what the marker records. The payload's argv[0] is
// the resolved path.
func gatedCommand(s JobSpec, workdir string, env []string) ([]string, bool, error) {
	if len(s.Command) == 0 {
		return nil, false, coded("E610", "empty command")
	}
	path, err := resolveGatedPath(s.Command[0], workdir, env)
	if err != nil {
		return nil, false, err
	}
	shell := gateShell()
	script := limitScript(s, runtime.GOOS) + "read -r g <&3 || exit 125; exec 3<&-; unset " + strings.Join(shellAdded, " ") + "; "
	var assigns string // the added names the job environment itself set, as env arguments
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		for _, name := range shellAdded {
			if k == name {
				script += e[:len(k)+1] + shellQuote(e[len(k)+1:]) + "; export " + k + "; "
				assigns += " " + shellQuote(e)
			}
		}
	}
	if shell == "sh" {
		// A bash sh re-exports SHLVL on every exec and cannot be told not to: hand over through env instead.
		script += `[ -z "$BASH_VERSION" ] || exec env -u SHLVL` + assigns + ` "$@"; `
	}
	return append([]string{shell, "-c", script + "exec \"$@\"", "ci-job", path}, s.Command[1:]...), true, nil
}

var (
	gateShellOnce sync.Once
	gateShellPath string
)

// gateShell prefers dash: it adds nothing to the environment it execs into and starts faster than bash.
func gateShell() string {
	gateShellOnce.Do(func() {
		gateShellPath = "sh"
		if p, err := osexec.LookPath("dash"); err == nil {
			gateShellPath = p
		}
	})
	return gateShellPath
}

// shellQuote single-quotes v for sh.
func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }

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
