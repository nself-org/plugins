package sample

// Purpose: the free-disk sample shared by every platform.
// Inputs: Sampler.DiskPath (default: the home directory) and the injected FreeBytes.
// Outputs: Live.DiskFreeMB, or a note saying why it is nil.
// Constraints: one filesystem only (the one that holds the CI home and its workspaces).

import "os"

// sampleDisk reads the free space of the filesystem under DiskPath.
func (s Sampler) sampleDisk(live *Live) {
	path := s.DiskPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			live.note("disk_free_mb", err)
			return
		}
		path = home
	}
	free := s.FreeBytes
	if free == nil {
		free = statfsFree
	}
	n, err := free(path)
	if err != nil {
		live.note("disk_free_mb", err)
		return
	}
	live.DiskFreeMB = i64(int64(n / (1 << 20)))
}
