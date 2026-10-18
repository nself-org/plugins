//go:build !linux

package hygiene

import "errors"

// isNoProc reports whether err means the host has no /proc.
func isNoProc(err error) bool { return errors.Is(err, errNoProc) }

// Without /proc the process and socket scans are not available; LeakCheck reports them as skipped
// and relies on the exec process marker and descendant tracking for the kill.

type procEntry struct {
	PID, PGID, UID int
	Cmd, Start     string
	Inodes         map[uint64]bool
}

type listener struct {
	Port  int
	UID   int
	Inode uint64
}

var errNoProc = errors.New("no /proc on this OS")

func scanProcs(string, int) ([]procEntry, error) { return nil, errNoProc }
func listeners() ([]listener, error)             { return nil, errNoProc }
func killProcs([]procEntry) error                { return nil }
