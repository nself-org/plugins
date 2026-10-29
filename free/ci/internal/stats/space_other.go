//go:build !(linux || darwin || freebsd)

package stats

// Purpose: free-space stand-in for platforms without statfs; the figure is reported as unknown (null).
// Inputs:  a path.
// Outputs: ok false.
// Constraints: none.

func freeBytes(string) (int64, bool) { return 0, false }
