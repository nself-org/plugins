//go:build !linux && !darwin

package run

// bootID is unknown on this platform; heartbeats alone decide staleness.
func bootID() string { return "boot-unknown" }
