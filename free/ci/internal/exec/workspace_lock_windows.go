//go:build windows

package exec

import "os"

// lockWorkspace reports busy on Windows, where stable workspaces are unsupported.
func lockWorkspace(*os.File) (bool, error) { return false, nil }

// unlockWorkspace is unused because Windows never acquires a workspace lock.
func unlockWorkspace(*os.File) {}
