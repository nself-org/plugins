//go:build !darwin

package specs

// cloneSupported is false: restores unpack directly, with no tree built first.
const cloneSupported = false

// cloneTree leaves restoration to unpacking on platforms without clonefile.
func cloneTree(src, dst string) error {
	return errCloneUnsupported
}
