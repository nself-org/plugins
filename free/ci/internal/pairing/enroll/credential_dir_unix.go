//go:build unix

package enroll

// Purpose: the owner, mode and symlink checks of the agent state dir (D-1259).
// Inputs:  the state dir and its pki subdirectory. Outputs: an error when either is not safe to hold keys.
// Constraints: the state dir (which may legitimately be reached through a symlink) must be owned by the
//          caller and not writable by group or other; the pki directory must be a real directory (never a
//          symlink) owned by the caller, and is opened without following links, so its chmod cannot land on
//          another target.

import (
	"fmt"
	"os"
	"syscall"
)

func checkStateOwner(state string) error {
	fi, err := os.Stat(state)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("enroll: the agent state %s is not a directory", state)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("enroll: the agent state %s is owned by uid %d, not the caller", state, st.Uid)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("enroll: the agent state %s is writable by group or other (mode %o)", state, fi.Mode().Perm())
	}
	return nil
}

// securePKIDir checks the pki directory without following a link and sets its mode to 0700.
func securePKIDir(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return fmt.Errorf("enroll: %s must be a real directory, not a link: %w", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("enroll: %s is owned by uid %d, not the caller", path, st.Uid)
	}
	return f.Chmod(0o700) // MkdirAll leaves an existing directory's mode alone
}
