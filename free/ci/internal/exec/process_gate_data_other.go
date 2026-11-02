//go:build !linux && !windows

package exec

// setDataLimit is never asked for off Linux (the ulimit wrapper set no memory cap there either).
func setDataLimit(uint64) error { return nil }
