//go:build windows

package cas

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("cas: the blob store is not supported on windows")

// oNoFollow is zero here: Windows has no O_NOFOLLOW. New refuses to build a Store, so nothing
// opens, writes or locks a blob tree on Windows. P7 supports no Windows runs; this file exists so
// the plugin cross-compiles for the release (CI Epic D35).
const oNoFollow = 0

// platformSupported fails closed: no Store can be created on Windows.
func platformSupported() error { return errUnsupported }

// flock is unreachable without a Store and refuses anyway.
func flock(*os.File) (func(), error) { return nil, errUnsupported }
