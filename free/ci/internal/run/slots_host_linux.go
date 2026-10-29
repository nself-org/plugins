//go:build linux

package run

import (
	"net"
	"os"
)

// Abstract Unix sockets are kernel-owned host leases shared by all OS users.
// They have no unlinkable filesystem entry and disappear when the holder dies.
// A package var so tests can lease from a private namespace instead of the real one.
var hostSlotDir = "@nself-ci/host-slots/v1"

func hostSlots() string { return hostSlotDir }

func lockHostSlot(n int) (*os.File, bool) {
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: hostSlotDir + "/" + itoa(n), Net: "unixgram"})
	if err != nil {
		return nil, false
	}
	f, err := c.File()
	_ = c.Close()
	return f, err == nil
}

func platformSlot(dir string, n int) (*os.File, bool, bool) {
	if dir != hostSlotDir {
		return nil, false, false
	}
	f, ok := lockHostSlot(n)
	return f, ok, true
}
