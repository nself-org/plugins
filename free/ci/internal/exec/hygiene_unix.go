//go:build !windows

package exec

// Purpose: the Unix half of the hygiene seam: kill the process group an attempt's marker names,
// and run the user-namespace probe through `unshare`.
// Inputs:  the CI home and attempt id (marker); a subordinate UID and a probe file (userns).
// Outputs: nil when the group is gone; the probe's exit class.
// Constraints: a group is killed only while the leader's start time still matches the marker (a
// reused PID is never signalled); the probe command line is built here from validated values only.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Probe exit codes of the in-namespace test: readable means the namespace does NOT separate.
const (
	usernsReadable = 10
	usernsDenied   = 11
)

// KillAttemptGroup kills the process group recorded for the attempt and removes its marker. An
// attempt with no marker, or whose group is already gone, is not an error.
func KillAttemptGroup(home, attempt string) error {
	if !safeID.MatchString(attempt) || home == "" {
		return coded("E610", "invalid attempt for process kill")
	}
	data, err := os.ReadFile(markerPath(home, attempt))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var marker processMarker
	if json.Unmarshal(data, &marker) != nil || marker.PID <= 0 {
		removeProcessMarker(home, attempt)
		return nil
	}
	killMarkedDescendants(marker.Descendants)
	if groupAlive(marker.PID) {
		same, known := sameProcess(marker.PID, marker.Identity)
		leaderGone := errors.Is(syscall.Kill(marker.PID, 0), syscall.ESRCH)
		switch {
		case leaderGone:
			// A live group whose leader has exited is the attempt's own: the kernel does not reuse a
			// PID that still names a process group.
		case !known:
			// the leader exists but its start time is unreadable: it cannot be told from a reused PID
			return coded("E610", "attempt process identity unavailable")
		case !same:
			// the leader PID now belongs to another process; its group is not ours to kill
			removeProcessMarker(home, attempt)
			return nil
		}
		if err := killGroup(marker.PID); err != nil && groupAlive(marker.PID) {
			return coded("E610", "attempt process group survived")
		}
		deadline := time.Now().Add(2 * time.Second)
		for groupAlive(marker.PID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if groupAlive(marker.PID) {
			return coded("E610", "attempt process group survived")
		}
	}
	removeProcessMarker(home, attempt)
	return nil
}

// UsernsArgv is the command prefix that runs a job as the subordinate host UID subUID: the namespace
// maps inner 0 to subUID, and --setuid/--setgid switch the process to it (without them it keeps its
// own, unmapped, UID and reads the agent's files). The probe verifies exactly this prefix.
func UsernsArgv(subUID int) []string {
	sub := strconv.Itoa(subUID)
	return []string{"unshare", "--user", "--map-users=0:" + sub + ":1", "--map-groups=0:" + sub + ":1", "--setuid=0", "--setgid=0", "--"}
}

// UsernsProbe runs UsernsArgv(subUID) around `sh -c 'test -r <file>'` and returns
// whether the namespace could read the file. ok is false when unshare itself could not set the
// namespace up; readable true means the mapped UID is NOT separated from the file's owner.
func UsernsProbe(ctx context.Context, subUID int, file string) (readable, ok bool, err error) {
	if subUID <= 0 || !filepath.IsAbs(file) || strings.ContainsAny(file, "\x00\n") {
		return false, false, coded("E610", "userns probe refused")
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	script := `if [ -r "$1" ]; then exit ` + strconv.Itoa(usernsReadable) + `; else exit ` + strconv.Itoa(usernsDenied) + `; fi`
	args := append(UsernsArgv(subUID), "sh", "-c", script, "probe", file)
	cmd := osexec.CommandContext(ctx, "unshare", args[1:]...)
	cmd.Env = dockerHostEnv()
	runErr := cmd.Run()
	var exit *osexec.ExitError
	switch {
	case runErr == nil:
		return false, false, nil
	case errors.As(runErr, &exit):
		switch exit.ExitCode() {
		case usernsReadable:
			return true, true, nil
		case usernsDenied:
			return false, true, nil
		}
		return false, false, nil
	default:
		return false, false, runErr
	}
}
