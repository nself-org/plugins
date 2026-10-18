//go:build windows

package exec

import "context"

// KillAttemptGroup is not supported on Windows (Epic D35).
func KillAttemptGroup(_, _ string) error { return coded("E610", "os_unsupported") }

// UsernsProbe is not supported on Windows (Epic D35).
func UsernsProbe(context.Context, int, string) (bool, bool, error) {
	return false, false, coded("E610", "os_unsupported")
}
