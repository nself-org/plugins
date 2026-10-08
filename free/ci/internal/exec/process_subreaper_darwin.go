//go:build darwin

package exec

import "syscall"

func ensureSubreaper() error                       { return nil }
func reapDescendants([]int)                        {}
func groupAlive(pid int) bool                      { return syscall.Kill(-pid, 0) == nil }
func adoptedDescendants(map[int]int, string) []int { return nil }
