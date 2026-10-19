//go:build !unix

// Purpose: the free-disk reader on a host without statfs(2).
// Inputs: a path. Outputs: always an error, so the disk value stays nil and its floor refuses.
// Constraints: never invents a number.

package sample

import "errors"

func statfsFree(string) (uint64, error) {
	return 0, errors.New("free disk is not readable on this platform")
}
