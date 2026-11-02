//go:build !linux && !darwin

package specs

// Purpose: platforms without a descriptor-based time call keep the time the
// file was written with (the restore sets modes, which Go cache trimming does
// not depend on). Constraints: never an error; a missing time is not a failure.

import (
	"os"
	"time"
)

func setTimes(*os.File, time.Time) error { return nil }
