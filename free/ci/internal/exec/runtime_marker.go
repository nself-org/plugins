package exec

// Purpose: remember which container runtimes this executor home has started a container with, so the
// orphan sweep lists only those (P7-CI-46). On a hosted Ubuntu runner `podman ps` resolves the package of
// six helper binaries through rpm and dpkg on every call (about 0.4 s each under strace), on a host where
// no job ever ran under podman.
// Inputs:  the executor home (JobSpec.Home, the home the job-start sweep reads), a runtime binary name.
// Outputs: markRuntimeUsed (before every labelled container start), sweepSkips (the sweep's filter),
//          markSweptAll (after one full sweep of every runtime).
// Constraints: fail closed. A container is started only after its runtime's marker is on disk, so every
//          orphan this home can remove sits in a runtime that has a marker. Until one full sweep of every
//          installed runtime has completed in the home (the .swept stamp), nothing is skipped: containers
//          left by a binary from before the markers existed are still found. Any error reading the markers
//          means "sweep it".

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	runtimeMarkerDir = "runtimes"
	sweptStamp       = ".swept"
)

// markRuntimeUsed records that binary is about to start a container for this home. An error refuses the start.
func markRuntimeUsed(home, binary string) error {
	if home == "" {
		return nil // no home: the sweep of this executor has no stamp either, so it skips nothing
	}
	if !runtimeBinary.MatchString(binary) {
		return coded("E613", "container runtime refused")
	}
	dir := filepath.Join(home, runtimeMarkerDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return coded("E613", "container runtime marker not written")
	}
	f, err := os.OpenFile(filepath.Join(dir, filepath.Base(binary)), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return coded("E613", "container runtime marker not written")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return coded("E613", "container runtime marker not written")
	}
	return f.Close()
}

// sweepSkips reports whether the sweep of home may skip binary: a full sweep has completed in the home and
// no container was ever started there with binary.
func sweepSkips(home, binary string) bool {
	if home == "" {
		return false
	}
	dir := filepath.Join(home, runtimeMarkerDir)
	if _, err := os.Stat(filepath.Join(dir, sweptStamp)); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, filepath.Base(binary)))
	return errors.Is(err, os.ErrNotExist)
}

// markSweptAll records that every installed runtime was swept for home without an error.
func markSweptAll(home string) {
	if home == "" {
		return
	}
	dir := filepath.Join(home, runtimeMarkerDir)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	if f, err := os.OpenFile(filepath.Join(dir, sweptStamp), os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		_ = f.Close()
	}
}
