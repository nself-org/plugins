//go:build windows

package exec

import "golang.org/x/sys/windows"

// stillActive is the exit code Windows reports for a process that has not exited (STILL_ACTIVE).
const stillActive = 259

// processAlive reports whether pid names a running process: one that opens and has no exit code yet.
func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if windows.GetExitCodeProcess(h, &code) != nil {
		return false
	}
	return code == stillActive
}
