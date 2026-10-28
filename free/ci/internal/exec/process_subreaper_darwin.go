//go:build darwin

package exec

import (
	"syscall"
	"time"
)

func ensureSubreaper() error    { return nil }
func reapDescendants([]int)     {}
func groupAlive(pid int) bool   { return syscall.Kill(-pid, 0) == nil }
func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// adoptedDescendants finds the job processes launchd adopted: macOS has no subreaper, so they are
// found by the attempt tag in their environment (process_tagged_darwin.go).
func adoptedDescendants(_ map[int]int, attempt string, since time.Time) []int {
	return taggedOrphans(attempt, since)
}
