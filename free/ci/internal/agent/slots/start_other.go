//go:build !linux && !darwin

package slots

// startTime is unavailable here; an owner is then matched by pid alone.
func startTime(int) string { return "" }
