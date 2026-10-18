package run

import (
	"os"
	"strings"
)

// bootID identifies this boot of the machine; a coordinator from another
// boot is stale.
func bootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "boot-unknown"
	}
	return "boot-" + strings.ReplaceAll(strings.TrimSpace(string(b)), "-", "")
}
