//go:build !unix

package records

import "io/fs"

// linkCount is 1: this platform does not report hard links here.
func linkCount(fs.FileInfo) uint64 { return 1 }
