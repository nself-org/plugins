package detach

// Purpose: the pid of a detached run's child, kept beside its event file
// (P7-CI-63): `runs follow` must not call a run over while its coordinator is
// still writing (the store marks a run finished before the last events go out).
// Inputs:  the stream file path and the child pid. Outputs: <stream>.pid (0600).
// Constraints: best effort (a missing file means "unknown"); the file is removed
// with its stream; the liveness check lives in pid_unix.go / pid_windows.go.

import (
	"os"
	"strconv"
	"strings"
)

// pidPath is the sidecar of a stream file.
func pidPath(stream string) string { return strings.TrimSuffix(stream, ".ndjson") + ".pid" }

// WritePID records the child pid beside the stream file.
func WritePID(stream string, pid int) error {
	return os.WriteFile(pidPath(stream), []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

// ReadPID returns the recorded pid, or 0 when none was recorded.
func ReadPID(stream string) int {
	b, err := os.ReadFile(pidPath(stream))
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n <= 1 {
		return 0
	}
	return n
}

// WriterAlive reports whether the child that writes this stream may still be
// running. Unknown (no pid recorded) counts as not alive: the caller then relies
// on the idle grace alone.
func WriterAlive(stream string) bool {
	pid := ReadPID(stream)
	return pid > 0 && pidAlive(pid)
}
