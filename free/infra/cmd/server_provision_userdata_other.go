//go:build !unix

// Purpose: non-unix stub of the --user-data-file reader. The no-follow open and
// the owner check need unix syscalls, so the flag is refused here (fail closed).
//
// Inputs: a path or FileInfo, both ignored.
//
// Outputs: always a coded usage error (E401).
//
// Constraints: keeps the windows build of the plugin compiling.
package main

import "os"

const userDataUnsupported = "--user-data-file is supported on Linux and macOS only"

// openUserDataFile always refuses on this platform.
func openUserDataFile(path string) (*os.File, error) {
	return nil, &usageError{msg: userDataUnsupported, safe: userDataUnsupported}
}

// checkUserDataOwner always refuses on this platform.
func checkUserDataOwner(fi os.FileInfo) error {
	return &usageError{msg: userDataUnsupported, safe: userDataUnsupported}
}
