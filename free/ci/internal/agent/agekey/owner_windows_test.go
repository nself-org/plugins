//go:build windows

package agekey

import (
	"io/fs"
	"os"
)

func fileOwner(fs.FileInfo) int { return os.Getuid() }
