//go:build !linux && !darwin

package specs

// Purpose: platforms without a descriptor-based time call (Windows, the BSDs).
// Constraints: setTimes always reports an error, so settleOpen tells the caller
// to set the time by path with root.Chtimes after closing the file, exactly as
// every restore did before the descriptor path existed.

import (
	"errors"
	"os"
	"time"
)

func setTimes(*os.File, time.Time) error { return errors.ErrUnsupported }
