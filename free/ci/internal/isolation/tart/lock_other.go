//go:build !unix

package tart

import (
	"errors"
	"os"
)

// tryLock is unsupported off unix; the profile is unavailable there (Tart runs on macOS only).
func tryLock(*os.File) (bool, error) { return false, errors.New("tart: file locks need a unix host") }
