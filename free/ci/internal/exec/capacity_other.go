//go:build !darwin

package exec

// probeDarwin is the macOS probe (capacity_darwin.go); Probe calls it only on darwin.
func probeDarwin(*Capacity) {}
