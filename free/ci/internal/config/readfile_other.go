//go:build !unix

package config

import "os"

// openNonBlocking opens path read-only; this platform has no FIFOs to block on.
func openNonBlocking(path string) (*os.File, error) { return os.Open(path) }
