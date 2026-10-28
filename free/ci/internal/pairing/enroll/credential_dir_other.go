//go:build !unix

package enroll

// Purpose: the state-dir checks where there is no owner or O_NOFOLLOW to ask for: a link is refused and
// the mode is set as before (D-1259).

import (
	"fmt"
	"os"
)

func checkStateOwner(string) error { return nil }

func securePKIDir(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("enroll: %s must be a real directory, not a link", path)
	}
	return os.Chmod(path, 0o700)
}
