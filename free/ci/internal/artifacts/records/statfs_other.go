//go:build !(linux || darwin || freebsd)

package records

import "errors"

// volumeSpace is unavailable here: the disk floor refuses writes (fail closed).
func volumeSpace(string) (uint64, uint64, error) {
	return 0, 0, errors.New("free space is not readable on this platform")
}
