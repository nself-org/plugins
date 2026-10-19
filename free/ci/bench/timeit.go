package bench

// Purpose: time one iteration of a scenario.
// Inputs:  the scenario function.
// Outputs: its wall time in milliseconds and its error.
// Constraints: wall clock only; no warm-up logic (the caller discards warm-up runs).

import "time"

// timeIt runs fn once and returns its wall time in milliseconds.
func timeIt(fn func() error) (float64, error) {
	start := time.Now()
	err := fn()
	return float64(time.Since(start).Microseconds()) / 1000, err
}
