//go:build darwin

package specs

import "golang.org/x/sys/unix"

// cloneSupported: APFS clones a tree copy-on-write in one call.
const cloneSupported = true

// cloneTree creates an independent copy-on-write tree without following links.
func cloneTree(src, dst string) error {
	return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
}
