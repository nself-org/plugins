//go:build windows

package backup

// Purpose: the data-dir lock on Windows: not implemented (the coordinator runs on unix hosts); the heartbeat
//          check of Restore still applies.
// Inputs: the data dir. Outputs: a no-op release.
// Constraints: none.

// Lock is a no-op on Windows.
func Lock(string) (release func(), err error) { return func() {}, nil }
