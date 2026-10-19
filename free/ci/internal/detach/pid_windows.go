//go:build windows

package detach

// pidAlive is not used on windows: detach is unsupported there (Epic D35).
func pidAlive(int) bool { return false }
