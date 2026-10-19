package sample

// Purpose: the free-disk sample shared by every platform.
// Inputs: Sampler.DiskPath (default: the CI home, or its nearest existing parent) and the
// injected FreeBytes.
// Outputs: Live.DiskFreeMB, or a note saying why it is nil.
// Constraints: one filesystem only: the one that holds the CI home and its workspaces,
// not the login home, which can sit on another volume.

import "errors"

// sampleDisk reads the free space of the filesystem under DiskPath.
func (s Sampler) sampleDisk(live *Live) {
	path := s.DiskPath
	if path == "" {
		// Jobs run under <CI home>/work, so that volume is the one that fills up.
		if path = CIHome(); path == "" {
			live.note("disk_free_mb", errors.New("no CI home to measure"))
			return
		}
		path = nearestExisting(path)
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
