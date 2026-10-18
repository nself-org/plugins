//go:build linux

package hygiene

// Purpose: the Linux /proc view LeakCheck and Cleanup use: processes of one attempt (by job UID or
// by the NSELF_CI_EXEC_ATTEMPT tag every job carries), TCP listeners, and the kill of those pids.
// Inputs:  an attempt id and a job UID (<= 0 when the job ran under the operator's own UID).
// Outputs: matching processes and listening sockets.
// Constraints: read-only except the SIGKILL; another UID's environ is unreadable and is skipped; the
// caller's own process is never returned.

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const procRoot = "/proc"

// procEntry is one process that belongs to an attempt.
type procEntry struct {
	PID, PGID, UID int
	Cmd            string
	// Start is the process start time (field 22 of /proc/<pid>/stat): with the PID it identifies one
	// process, so a PID reused after the scan is never signalled.
	Start  string
	Inodes map[uint64]bool
}

// isNoProc reports whether err means the host has no /proc (never true on Linux).
func isNoProc(error) bool { return false }

// scanProcs returns the live processes owned by uid (when uid > 0) or tagged with the attempt id.
func scanProcs(attempt string, uid int) ([]procEntry, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	tag := []byte("NSELF_CI_EXEC_ATTEMPT=" + attempt + "\x00")
	self := os.Getpid()
	var out []procEntry
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		dir := filepath.Join(procRoot, e.Name())
		status, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil {
			continue
		}
		puid, state, name := parseStatus(string(status))
		if state == 'Z' || state == 'X' {
			continue
		}
		match := uid > 0 && puid == uid
		if !match && attempt != "" {
			if env, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil && containsTag(env, tag) {
				match = true
			}
		}
		if !match {
			continue
		}
		pg, start := statFields(dir)
		out = append(out, procEntry{PID: pid, PGID: pg, UID: puid, Cmd: name, Start: start, Inodes: socketInodes(dir)})
	}
	return out, nil
}

func containsTag(env, tag []byte) bool {
	return bytes.Contains(append([]byte{0}, env...), append([]byte{0}, tag...))
}

func parseStatus(s string) (uid int, state byte, name string) {
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, "Uid:"):
			if f := strings.Fields(line); len(f) > 1 {
				uid, _ = strconv.Atoi(f[1])
			}
		case strings.HasPrefix(line, "State:"):
			if f := strings.Fields(line); len(f) > 1 {
				state = f[1][0]
			}
		case strings.HasPrefix(line, "Name:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
		}
	}
	return uid, state, name
}

// statFields returns the process group and start time of a /proc/<pid> directory.
func statFields(dir string) (pgid int, start string) {
	data, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return 0, ""
	}
	s := string(data)
	if i := strings.LastIndexByte(s, ')'); i >= 0 {
		// fields after the command: state ppid pgrp session ... starttime is field 22 overall
		f := strings.Fields(s[i+1:])
		if len(f) > 19 {
			pgid, _ = strconv.Atoi(f[2])
			start = f[19]
		}
	}
	return pgid, start
}

func socketInodes(dir string) map[uint64]bool {
	fds, err := os.ReadDir(filepath.Join(dir, "fd"))
	if err != nil {
		return nil
	}
	out := map[uint64]bool{}
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(dir, "fd", fd.Name()))
		if err != nil || !strings.HasPrefix(target, "socket:[") {
			continue
		}
		if n, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]"), 10, 64); err == nil {
			out[n] = true
		}
	}
	return out
}

// listener is one TCP socket in LISTEN state.
type listener struct {
	Port  int
	UID   int
	Inode uint64
}

// listeners reads /proc/net/tcp and tcp6.
func listeners() ([]listener, error) {
	var out []listener
	for _, name := range []string{"tcp", "tcp6"} {
		f, err := os.Open(filepath.Join(procRoot, "net", name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 || fields[3] != "0A" {
				continue
			}
			port := 0
			if i := strings.LastIndexByte(fields[1], ':'); i >= 0 {
				p, _ := strconv.ParseUint(fields[1][i+1:], 16, 32)
				port = int(p)
			}
			uid, _ := strconv.Atoi(fields[7])
			inode, _ := strconv.ParseUint(fields[9], 10, 64)
			out = append(out, listener{Port: port, UID: uid, Inode: inode})
		}
		_ = f.Close()
	}
	return out, nil
}

// killProcs sends SIGKILL to each process through a pidfd (os.FindProcess pins the process), after
// checking its start time still matches the scan, so a PID reused in between is left alone. A
// process that is already gone is not an error; a refusal (EPERM) is.
func killProcs(procs []procEntry) error {
	var failed []string
	for _, p := range procs {
		proc, err := os.FindProcess(p.PID)
		if err != nil {
			continue
		}
		if _, start := statFields(filepath.Join(procRoot, strconv.Itoa(p.PID))); start != p.Start {
			_ = proc.Release()
			continue
		}
		if err := proc.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
			failed = append(failed, "kill pid "+strconv.Itoa(p.PID)+": "+err.Error())
		}
		_ = proc.Release()
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}
