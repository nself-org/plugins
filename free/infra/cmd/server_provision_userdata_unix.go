//go:build unix

// Purpose: unix half of the --user-data-file reader: the no-follow open and the
// owner check that need syscall.O_NOFOLLOW and syscall.Stat_t.
//
// Inputs: a path (open) or the FileInfo of the already open descriptor (owner).
//
// Outputs: an open *os.File, or a coded usage error (E401) with no content.
//
// Constraints: O_NOFOLLOW makes a symlink fail with ELOOP; O_NONBLOCK keeps a
// FIFO from hanging the open (the regular-file check then refuses it);
// O_NOCTTY keeps a terminal device from becoming the controlling terminal.
package main

import (
	"errors"
	"os"
	"syscall"
)

// userDataUID is the UID the file must belong to; a variable so tests can
// simulate another owner.
var userDataUID = os.Getuid

// openUserDataFile opens path read-only without following a final symlink.
func openUserDataFile(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err == nil {
		return f, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		return nil, userDataRefusal(path, "is a symbolic link")
	}
	return nil, userDataOpenError(path, err)
}

// checkUserDataOwner refuses a file the invoking user does not own.
func checkUserDataOwner(fi os.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != userDataUID() {
		return errors.New("not owned by the invoking user")
	}
	return nil
}
