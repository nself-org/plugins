//go:build !windows

package main

// Purpose: the trust check of `uninstall --system` (P7-NODE-31, review R1): the agent binary under
// /opt/nself-ci-agent belongs to the agent user, so a root shell must not run it; uninstall may run as
// root only from a file whose whole path belongs to root and cannot be changed by anyone else.
// Inputs:  the path of the running executable.
// Outputs: nil when the file and every directory above it (after symlinks) is owned by root and not
//          writable by group or others; otherwise an error naming the first path that fails.
// Constraints: unix only (windows has no uid model and refuses in uninstall_trust_windows.go).

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func trustedRootPath(path string) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	for p := real; ; p = filepath.Dir(p) {
		fi, lerr := os.Lstat(p)
		if lerr != nil {
			return lerr
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 {
			return fmt.Errorf("%s is not owned by root", p)
		}
		if fi.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s can be changed by its group or by others", p)
		}
		if p == filepath.Dir(p) {
			return nil
		}
	}
}
