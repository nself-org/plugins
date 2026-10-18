package run

import (
	"encoding/hex"
	"syscall"
)

// bootID identifies this boot of the machine; a coordinator from another
// boot is stale.
func bootID() string {
	raw, err := syscall.Sysctl("kern.boottime")
	if err != nil || raw == "" {
		return "boot-unknown"
	}
	return "boot-" + hex.EncodeToString([]byte(raw))
}
