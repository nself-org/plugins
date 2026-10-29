//go:build !linux

package run

import "os"

// Rootful Linux Docker execution needs host-wide leases. Other platforms keep
// their existing home-scoped locks (Docker on those platforms runs in a VM).
func hostSlots() string { return "" }

func platformSlot(string, int) (*os.File, bool, bool) { return nil, false, false }
