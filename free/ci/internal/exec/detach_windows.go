//go:build windows

package exec

// Purpose: windows stub of SpawnDetached (P7-CI-63, Epic D35): detach is not
// supported, so it fails with E648 reason spawn and the package still builds.

import "os/exec"

// applyDetach reports that detach is unsupported on windows.
func applyDetach(*exec.Cmd) error {
	return &Error{Code: detachCode, Message: "spawn: " + errNoDetach.Error(), Err: errNoDetach}
}

// TerminateDetached is not supported on windows.
func TerminateDetached(int) error {
	return &Error{Code: detachCode, Message: "spawn: " + errNoDetach.Error(), Err: errNoDetach}
}
