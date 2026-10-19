//go:build windows

package lifecycle

// lockNode is a no-op on Windows: the CI agent and the node commands run on Linux and macOS only.
func (d *Deps) lockNode(string) (func(), error) { return func() {}, nil }
