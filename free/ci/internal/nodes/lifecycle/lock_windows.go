//go:build windows

package lifecycle

// The node commands run on Linux and macOS only; the locks are no-ops on Windows.
func (d *Deps) lockNode(string) (func(), error) { return func() {}, nil }
func (d *Deps) lockPins() (func(), error)       { return func() {}, nil }
