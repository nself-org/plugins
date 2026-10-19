//go:build !windows

package agekey

import (
	"io/fs"
	"syscall"
)

func fileOwner(info fs.FileInfo) int { return int(info.Sys().(*syscall.Stat_t).Uid) }
