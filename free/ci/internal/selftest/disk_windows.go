//go:build windows

package selftest

import "errors"

// FreeMB is not available on this platform; the disk check then warns.
func FreeMB(string) (int64, error) { return 0, errors.New("free space is not read on this platform") }
