//go:build race

package stats

// raceEnabled is true when the race detector is on, which slows the 100 000-attempt budget test by an order of magnitude.
const raceEnabled = true
