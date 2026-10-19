package contention

// Purpose: notice that the machine slept, so an intermittent node can drop leases the coordinator has
// already expired (Gate.OnWake).
// Inputs:  a wall clock and a monotonic clock (both injectable).
// Outputs: Tick reports true once per sleep.
// Constraints: the monotonic clock does not advance while the machine sleeps on every supported OS
//          (Go's monotonic reading is CLOCK_MONOTONIC on Linux and mach_absolute_time on macOS), the
//          wall clock does; a gap beyond Threshold is a sleep.

import "time"

// SleepWatcher compares the wall clock with the monotonic clock between ticks.
type SleepWatcher struct {
	// Threshold is the smallest gap counted as a sleep; zero means 10 s.
	Threshold time.Duration
	// Wall and Mono default to time.Now and a monotonic reading of it.
	Wall func() time.Time
	Mono func() time.Duration

	started  bool
	lastWall time.Time
	lastMono time.Duration
	monoBase time.Time
}

func (w *SleepWatcher) now() (time.Time, time.Duration) {
	wall := time.Now
	if w.Wall != nil {
		wall = w.Wall
	}
	if w.Mono != nil {
		return wall(), w.Mono()
	}
	if w.monoBase.IsZero() {
		w.monoBase = time.Now()
	}
	return wall().Round(0), time.Since(w.monoBase)
}

// Tick returns true when the wall clock moved further than the monotonic clock since the last call.
func (w *SleepWatcher) Tick() bool {
	wall, mono := w.now()
	slept := false
	if w.started {
		threshold := w.Threshold
		if threshold <= 0 {
			threshold = 10 * time.Second
		}
		slept = wall.Sub(w.lastWall)-(mono-w.lastMono) > threshold
	}
	w.started, w.lastWall, w.lastMono = true, wall, mono
	return slept
}
