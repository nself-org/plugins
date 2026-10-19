//go:build !unix

package readiness

import (
	"errors"
	"os"
)

func flock(*os.File) error { return errors.New("the run lock needs a unix host") }

func isBusy(error) bool { return false }
