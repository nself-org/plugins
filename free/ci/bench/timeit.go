package bench

import "time"

// timeIt runs fn once and returns its wall time in milliseconds.
func timeIt(fn func() error) (float64, error) {
	start := time.Now()
	err := fn()
	return float64(time.Since(start).Microseconds()) / 1000, err
}
