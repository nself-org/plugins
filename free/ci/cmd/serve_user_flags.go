package main

// Purpose: the serve flag snapshot the V15 user listeners read (P7-NODE-19, D-1406): the flag set captured by
// the listener flag registration, and the immutable copy taken once per serve.
// Inputs:  the serve flag set registered in serve_user_listener.go.
// Outputs: serveFlag and explicitFlag, stable values for listener goroutines.
// Constraints: the snapshot is process-global; serves in one process must be serialized.

import (
	"flag"
	"sync"
)

// userAPIFlags is the serve flag set the listeners read --data-dir and --api-public-url from.
var userAPIFlags struct {
	sync.Mutex
	fs *flag.FlagSet
}

// serveFlagSnapshot is a copy of the parsed serve flags taken once at listener
// start under userAPIFlags' lock, so listener goroutines never read the
// package-level flag set that a later test's serveV15 may replace (D-1406).
type serveFlagSnapshot struct {
	dataDir       string
	apiPublicURL  string
	apiSubmitRole string
	explicit      map[string]bool
}

var activeServeFlags struct {
	sync.Mutex
	snap *serveFlagSnapshot
}

// serveFlag reads a parsed serve flag from the snapshot taken at listener start (D-1406).
func serveFlag(name string) string {
	activeServeFlags.Lock()
	defer activeServeFlags.Unlock()
	if activeServeFlags.snap == nil {
		return ""
	}
	switch name {
	case "data-dir":
		return activeServeFlags.snap.dataDir
	case "api-public-url":
		return activeServeFlags.snap.apiPublicURL
	case "api-submit-role":
		return activeServeFlags.snap.apiSubmitRole
	}
	return ""
}

// explicitFlag reports whether the operator passed the serve flag, from the snapshot (D-1406).
func explicitFlag(name string) bool {
	activeServeFlags.Lock()
	defer activeServeFlags.Unlock()
	if activeServeFlags.snap == nil {
		return false
	}
	return activeServeFlags.snap.explicit[name]
}

// snapshotServeFlags captures the current serve flag set under lock so
// listener goroutines read stable values even when another serveV15 call
// replaces userAPIFlags.fs (D-1406). Call once per serve, before listeners start.
// NOTE: activeServeFlags.snap is process-global; concurrent serves in the same
// process overwrite each other's snapshot. This matches the existing single-serve
// contract and is acceptable for tests that serialize serveV15 calls.
func snapshotServeFlags() {
	userAPIFlags.Lock()
	fs := userAPIFlags.fs
	userAPIFlags.Unlock()
	snap := &serveFlagSnapshot{explicit: map[string]bool{}}
	if fs != nil {
		if f := fs.Lookup("data-dir"); f != nil {
			snap.dataDir = f.Value.String()
		}
		if f := fs.Lookup("api-public-url"); f != nil {
			snap.apiPublicURL = f.Value.String()
		}
		if f := fs.Lookup("api-submit-role"); f != nil {
			snap.apiSubmitRole = f.Value.String()
		}
		fs.Visit(func(f *flag.Flag) { snap.explicit[f.Name] = true })
	}
	activeServeFlags.Lock()
	activeServeFlags.snap = snap
	activeServeFlags.Unlock()
}
