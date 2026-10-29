//go:build !unix

package verify

// Purpose: the open where there is no O_NOFOLLOW or O_NONBLOCK to ask for (D-1198). The caller's
// Lstat of every part and fstat of the opened file are the only checks here.

import "os"

// openRegular opens path read-only; follow is not used.
func openRegular(path string, _ bool) (*os.File, error) { return os.Open(path) }
