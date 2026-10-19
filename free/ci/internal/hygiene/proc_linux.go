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
	"sync"
	"sync/atomic"
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
// Each pid is judged on its own goroutine (a busy host has hundreds of them and the scan sits on the
// path of every job's end, D-1008); the result keeps /proc's order.
func scanProcs(attempt string, uid int) ([]procEntry, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	tag := []byte("NSELF_CI_EXEC_ATTEMPT=" + attempt + "\x00")
	self := os.Getpid()
	pids := make([]int, 0, len(entries))
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil && pid != self {
			pids = append(pids, pid)
		}
	}
	found := make([]*procEntry, len(pids))
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(scanWorkers, len(pids)) {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= len(pids) {
					return
				}
				found[i] = scanOne(pids[i], attempt, uid, tag)
			}
		})
	}
	wg.Wait()
	var out []procEntry
	for _, p := range found {
		if p != nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

// scanWorkers bounds the goroutines of one scan.
const scanWorkers = 8

// scanOne returns the entry for pid when it belongs to the attempt, else nil. Only a match pays for
// the stat reads: it needs the start time before and after the identifying reads, so a PID reused
// meanwhile (fields of two processes) is dropped.
func scanOne(pid int, attempt string, uid int, tag []byte) *procEntry {
	dir := filepath.Join(procRoot, strconv.Itoa(pid))
	matches := func() (puid int, name string, ok bool) {
		status, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil {
			return 0, "", false
		}
		puid, state, name := parseStatus(string(status))
		if state == 'Z' || state == 'X' {
			return 0, "", false
		}
		if uid > 0 && puid == uid {
			return puid, name, true
		}
		if attempt != "" {
			if env, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil && containsTag(env, tag) {
				return puid, name, true
			}
		}
		return 0, "", false
	}
	if _, _, ok := matches(); !ok {
		return nil
	}
	_, start := statFields(dir)
	puid, name, ok := matches() // judged again once the start time is known: the match is of that process
	if !ok || start == "" {
		return nil
	}
	pg, again := statFields(dir)
	if again != start {
		return nil
	}
	return &procEntry{PID: pid, PGID: pg, UID: puid, Cmd: name, Start: start, Inodes: socketInodes(dir)}
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
