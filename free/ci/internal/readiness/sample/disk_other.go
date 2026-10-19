//go:build !unix

package sample

import "errors"

func statfsFree(string) (uint64, error) {
	return 0, errors.New("free disk is not readable on this platform")
}
