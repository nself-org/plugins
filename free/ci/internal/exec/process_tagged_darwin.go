//go:build darwin

package exec

// Purpose: find job processes that escaped the group and the parent chain on macOS (D-0456, D-1057).
// A job step that daemonizes (fork, setsid, parent exits) is reparented to launchd, and macOS has no
// child subreaper to catch it as Linux does. The job environment carries NSELF_CI_EXEC_ATTEMPT=<attempt>,
// which a daemon inherits, so a launchd child of this user that holds the tag belongs to the attempt.
// Constraints: only processes of the agent's own user can have their environment read; the table is
// read once, and the environment only for launchd children that started after `since` (reading all of
// them costs about 4 ms on a busy desktop, too much for a 50 ms tracker tick).

import (
	"bytes"
	"encoding/binary"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// launchdPID is the parent of every process whose own parent exited.
const launchdPID = 1

// taggedOrphans returns the pids of launchd children of this user, started at or after since, whose
// environment carries the tag of attempt. A zero since looks at every launchd child.
func taggedOrphans(attempt string, since time.Time) []int {
	if attempt == "" {
		return nil
	}
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	uid := uint32(os.Getuid())
	var found []int
	for i := range procs {
		p := &procs[i]
		pid := int(p.Proc.P_pid)
		if pid <= launchdPID || int(p.Eproc.Ppid) != launchdPID || p.Eproc.Ucred.Uid != uid {
			continue
		}
		if !since.IsZero() && time.Unix(int64(p.Proc.P_starttime.Sec), int64(p.Proc.P_starttime.Usec)*1000).Before(since) {
			continue
		}
		if raw, err := unix.SysctlRaw("kern.procargs2", pid); err == nil && environHas(raw, "NSELF_CI_EXEC_ATTEMPT="+attempt) {
			found = append(found, pid)
		}
	}
	return found
}

// environHas reports whether the kern.procargs2 buffer of a process holds entry among its environment
// strings. The buffer is argc (int32), the executable path, NUL padding, argc argument strings, then
// the environment strings; an argument that happens to equal entry does not count.
func environHas(raw []byte, entry string) bool {
	if len(raw) < 4 {
		return false
	}
	argc := int(int32(binary.LittleEndian.Uint32(raw)))
	rest := raw[4:]
	if i := bytes.IndexByte(rest, 0); i >= 0 {
		rest = rest[i:]
	} else {
		return false
	}
	rest = bytes.TrimLeft(rest, "\x00")
	for n := 0; n < argc; n++ {
		i := bytes.IndexByte(rest, 0)
		if i < 0 {
			return false
		}
		rest = rest[i+1:]
	}
	for _, kv := range bytes.Split(rest, []byte{0}) {
		if string(kv) == entry {
			return true
		}
	}
	return false
}
