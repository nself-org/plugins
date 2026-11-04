//go:build windows

package main

// Purpose: the job-shim ops have no Windows family (no job users there): both checks refuse.

import (
	"errors"
	"io/fs"
)

func killOwnProcesses() error { return errors.New("job-shim ops are not supported on windows") }

func ownedBySelf(fs.FileInfo) bool { return false }
