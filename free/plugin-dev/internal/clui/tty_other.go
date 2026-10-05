//go:build !linux && !darwin

package clui

// isTerminalFd is false on platforms the plugin does not ship for.
func isTerminalFd(fd uintptr) bool { return false }
