package run

import (
	"encoding/hex"
	"syscall"
)

// bootID identifies this boot of the machine; a coordinator from another
// boot is stale. kern.bootsessionuuid is stable for the whole boot;
// kern.boottime moves when the clock is stepped, so it is only the fallback.
func bootID() string {
	if raw, err := syscall.Sysctl("kern.bootsessionuuid"); err == nil && raw != "" {
		return "boot-" + raw
	}
	raw, err := syscall.Sysctl("kern.boottime")
	if err != nil || raw == "" {
		return "boot-unknown"
	}
	return "boot-" + hex.EncodeToString([]byte(raw))
}
