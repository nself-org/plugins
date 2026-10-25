package config

// Purpose: read a config or legacy manifest without trusting the path.
// Inputs:  a file path inside a repository that may be hostile.
// Outputs: the file bytes, or an error for anything that is not a regular
// file or exceeds MaxFileBytes.
// Constraints: a symlink is followed only to a regular file; a device (a link
// to /dev/zero) or a FIFO is refused before it is opened, so a read can
// neither fill memory nor block forever. The path check is only an early
// exit: the file is opened without blocking and its type is checked again on
// the open descriptor, so a swap between the check and the open cannot hang.

import (
	"fmt"
	"io"
	"os"
)

// MaxFileBytes bounds a config or legacy manifest.
const MaxFileBytes = 1 << 20

// testHookBeforeOpen runs between the path check and the open; tests use it
// to swap the file, as a concurrent local writer could.
var testHookBeforeOpen = func(string) {}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if info, err = os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%s is a dangling symlink", path)
			}
			return nil, err
		}
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	testHookBeforeOpen(path)
	f, err := openNonBlocking(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if fi, err := f.Stat(); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, MaxFileBytes)
	}
	return data, nil
}
