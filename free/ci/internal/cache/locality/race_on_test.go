//go:build race

package locality

// raceSlowdown scales the wall-clock budget: the race detector slows the contributor roughly tenfold.
const raceSlowdown = 10
