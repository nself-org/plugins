package run

// pidAlive is conservative on Windows: heartbeats decide staleness there.
func pidAlive(pid int) bool { return pid > 0 }
