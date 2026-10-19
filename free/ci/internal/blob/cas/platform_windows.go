//go:build windows

package cas

import (
	"errors"
	"os"
)

// oNoFollow is zero here: Windows has no O_NOFOLLOW. The store refuses every mutation first
// (flock below fails), and reads keep the Lstat and SameFile checks. P7 supports no Windows runs;
// this file exists so the plugin cross-compiles for the release (CI Epic D35).
const oNoFollow = 0

// flock reports that the blob store cannot be locked on Windows, so no tree mutation can run there.
func flock(*os.File) (func(), error) {
	return nil, errors.New("cas: the blob store is not supported on windows")
}
