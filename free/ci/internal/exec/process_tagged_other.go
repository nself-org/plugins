//go:build !darwin

package exec

import "time"

// taggedOrphans is darwin-only: Linux keeps an escaped descendant reachable through the child
// subreaper (adoptedDescendants), and windows has no process tree here.
func taggedOrphans(string, time.Time) []int { return nil }
