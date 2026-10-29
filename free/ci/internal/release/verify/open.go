package verify

// Purpose: open a release file for reading without trusting an earlier look at it (D-1198).
// Inputs:  the release directory and a path from the manifest, or an operator trust file path.
// Outputs: an open regular file, or an error.
// Constraints: the Lstat of safePath is a first look only; the open refuses a link and never blocks
// (openRegular), and the mode is read from the opened descriptor.

import (
	"fmt"
	"os"
)

// open opens the regular file rel under dir. safePath's Lstat is only a first look: a local writer can
// swap the leaf for a symbolic link or a FIFO before the open, so the open itself refuses a link and
// never blocks (openRegular), and the mode is read from the opened descriptor (D-1198).
func open(dir, rel string) (*os.File, error) {
	p, err := safePath(dir, rel)
	if err != nil {
		return nil, err
	}
	return openChecked(p, rel, false)
}

// openChecked opens path and refuses anything but a regular file, judged on the descriptor itself.
func openChecked(path, name string, follow bool) (*os.File, error) {
	f, err := openRegular(path, follow)
	if err != nil {
		return nil, err
	}
	if st, serr := f.Stat(); serr != nil || !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%q is not a regular file", name)
	}
	return f, nil
}
