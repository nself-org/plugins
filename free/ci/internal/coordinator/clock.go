package coordinator

// Purpose: the time source of the coordinator's schedules and maintenance (moved out of server.go to keep it
// inside the file-size budget).
// Inputs:  none. Outputs: Clock and its real implementation. Constraints: tests inject a fake Clock.

import "time"

// Clock is the time source of the schedules; tests inject a fake.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }
