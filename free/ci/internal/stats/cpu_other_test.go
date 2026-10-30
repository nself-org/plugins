//go:build !unix

package stats

import "time"

// cpuTime has no per-process CPU clock here, so it reads the wall clock.
func cpuTime() time.Duration { return time.Duration(time.Now().UnixNano()) }
