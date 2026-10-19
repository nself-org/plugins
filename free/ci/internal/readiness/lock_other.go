//go:build !unix

// Purpose: the run lock on a host without flock(2). The watcher is unix-only (AUTO A6), so
// there the lock always refuses rather than pretending to exclude a second watcher.
// Inputs: an open file. Outputs: always an error; isBusy is always false.
// Constraints: never fail-open.

package readiness

import (
	"errors"
	"os"
)

func flock(*os.File) error { return errors.New("the run lock needs a unix host") }

func isBusy(error) bool { return false }
