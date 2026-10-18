//go:build windows

package run

import "os"

// lockSlot has no cross-process lock on Windows (container isolation is Linux and macOS only): the slot is process-local.
func lockSlot(string, int) (*os.File, bool) { return nil, true }
