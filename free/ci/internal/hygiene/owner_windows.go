//go:build windows

package hygiene

import "io/fs"

func ownedByCaller(fs.FileInfo) bool { return true }
