//go:build windows

package backup

// Purpose: the default liveness probe of a coordinator process id on Windows.
// Inputs: a pid. Outputs: true for any positive pid (fail closed: the heartbeat window decides).
// Constraints: none.

func processAlive(pid int) bool { return pid > 0 }
