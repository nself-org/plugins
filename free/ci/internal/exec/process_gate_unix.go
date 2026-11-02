//go:build !windows

package exec

// Purpose: the start-gate helper (D-1531). A gated job starts as the agent's
// own binary run with GateArg, not as a shell: it waits on fd 3 for the agent's
// go-ahead (sent only once the process marker is on disk), applies the job's
// rlimits, then replaces itself with the payload.
// Inputs:  argv = [exe, GateArg, cpuSeconds, dataKiB, path, payload argv...]
// ("0" means no such limit); fd 3 is the gate pipe; the environment is the
// payload's environment and is handed on untouched.
// Outputs: the payload, in this same process (same pid), or exit 125 when the
// gate closed without a go-ahead or a limit could not be set, 126 when the
// payload could not be executed.
// Constraints: the helper must not touch signals or the environment before the
// exec (an ignored signal or a changed variable would reach the payload), and
// must be the first thing the agent's main runs.

import (
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// GateArg is the first argument that makes the agent binary act as the start-gate helper.
const GateArg = "--nself-ci-start-gate"

// GateRequested reports whether args (os.Args) ask for the helper.
func GateRequested(args []string) bool { return len(args) >= 6 && args[1] == GateArg }

// gateExe is how a gated job re-executes the agent. On Linux /proc/self/exe names the running binary even
// when an upgrade has replaced the file on disk.
func gateExe() (string, error) {
	if runtime.GOOS == "linux" {
		return "/proc/self/exe", nil
	}
	return os.Executable()
}

// gatedCommand is the job's argv with the start gate. The payload is resolved here, in the agent, so a
// missing binary is a start error with the class a direct start has. The limits are the ones the ulimit
// wrapper set (limitedCommandOn): `ulimit -t N` sets RLIMIT_CPU soft and hard to N seconds, and on Linux
// `ulimit -d K` sets RLIMIT_DATA soft and hard to K KiB; the helper sets the same values with setrlimit.
func gatedCommand(s JobSpec, workdir string, env []string) ([]string, bool, error) {
	if len(s.Command) == 0 {
		return nil, false, coded("E610", "empty command")
	}
	path, err := resolveGatedPath(s.Command[0], workdir, env)
	if err != nil {
		return nil, false, err
	}
	exe, err := gateExe()
	if err != nil {
		return nil, false, err
	}
	cpu, data := 0, 0
	if s.Timeout > 0 {
		cpu = int((s.Timeout + time.Second - 1) / time.Second)
	}
	if s.MemoryLimitMB > 0 && runtime.GOOS == "linux" {
		data = s.MemoryLimitMB * 1024
	}
	argv := []string{exe, GateArg, strconv.Itoa(cpu), strconv.Itoa(data), path}
	return append(argv, s.Command...), true, nil
}

// RunGate runs the helper and returns its exit code; it returns only on failure (a success execs away).
func RunGate(args []string) int {
	cpu, err1 := strconv.ParseUint(args[2], 10, 63)
	data, err2 := strconv.ParseUint(args[3], 10, 53)
	if err1 != nil || err2 != nil {
		return 2
	}
	path, argv := args[4], args[5:]
	// The go-ahead is one line. End of file before it (the agent died, or gave up) or a closed fd 3: run nothing.
	var b [1]byte
	gate := os.NewFile(3, "gate")
	for {
		n, err := gate.Read(b[:])
		if n == 1 && b[0] == '\n' {
			break
		}
		if err != nil {
			return 125
		}
	}
	_ = syscall.Close(3)
	if cpu > 0 {
		if syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: cpu, Max: cpu}) != nil {
			return 125
		}
	}
	if data > 0 {
		if setDataLimit(data*1024) != nil {
			return 125
		}
	}
	err := syscall.Exec(path, argv, os.Environ())
	_, _ = os.Stderr.WriteString("error: exec " + path + ": " + err.Error() + "\n")
	return 126
}
