//go:build windows

package cas

import (
	"context"
	"errors"
	"os"
)

// noFollow is zero: windows has no O_NOFOLLOW. Final-symlink refusal still
// holds because every open is paired with an os.Root Lstat and SameFile check.
const noFollow = 0

// lockFile is a stub: windows builds exist (P7-PLUG-69) but the store does not
// run there, so locking reports errors.ErrUnsupported.
func lockFile(*os.File) error { return errors.ErrUnsupported }

func lockFileCtx(context.Context, *os.File) error { return errors.ErrUnsupported }

func unlockFile(*os.File) {}
