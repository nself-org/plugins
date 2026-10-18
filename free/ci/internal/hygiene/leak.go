package hygiene

// Purpose: LeakCheck, the end-of-attempt proof that nothing of the attempt survived. The caller
// (run hook or agent) drains the node when the report is not clean.
// Inputs:  an attempt id and the attempt's job UID (<= 0 when it ran under the operator's own UID,
// where a UID scan would match the operator's whole session).
// Outputs: a Report; E681 (hygiene.leak_detected) when it is not clean.
// Constraints: read-only; a check that could not run is reported as an "unverified" leak, never as
// clean; without /proc (macOS) the process and socket scans are reported as skipped.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LeakKind says what survived.
type LeakKind string

// The kinds of leftover LeakCheck reports.
const (
	LeakRoot       LeakKind = "root"
	LeakProcess    LeakKind = "process"
	LeakSocket     LeakKind = "socket"
	LeakFile       LeakKind = "file"
	LeakContainer  LeakKind = "container"
	LeakNetwork    LeakKind = "network"
	LeakVolume     LeakKind = "volume"
	LeakUnverified LeakKind = "unverified"
)

// Leak is one leftover.
type Leak struct {
	Kind   LeakKind `json:"kind"`
	Detail string   `json:"detail"`
}

// Report is the outcome of one LeakCheck.
type Report struct {
	Attempt string   `json:"attempt"`
	UID     int      `json:"uid,omitempty"`
	Leaks   []Leak   `json:"leaks,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
}

// Clean reports whether nothing survived.
func (r Report) Clean() bool { return len(r.Leaks) == 0 }

// Reason is the one-line text a drain callback records.
func (r Report) Reason() string {
	parts := make([]string, 0, len(r.Leaks))
	for _, l := range r.Leaks {
		parts = append(parts, string(l.Kind)+" "+l.Detail)
	}
	return fmt.Sprintf("attempt %s left %d leftovers: %s", r.Attempt, len(r.Leaks), strings.Join(parts, "; "))
}

// LeakCheck inspects the host for anything of the attempt that survived Cleanup.
func (h *Host) LeakCheck(ctx context.Context, attempt string, uid int) (Report, error) {
	rep := Report{Attempt: attempt, UID: uid}
	if !ValidAttempt(attempt) {
		return rep, leakError(ReasonBadAttempt, "invalid attempt id")
	}
	root := ""
	if h.Base != "" {
		root = filepath.Join(h.Base, attempt)
		if _, err := os.Lstat(root); err == nil {
			rep.Leaks = append(rep.Leaks, Leak{LeakRoot, root})
		} else if !os.IsNotExist(err) {
			rep.Leaks = append(rep.Leaks, Leak{LeakUnverified, "job root: " + firstLine(err.Error())})
		}
	}
	h.checkProcs(&rep, attempt, uid)
	if uid > 0 && uid != os.Geteuid() {
		dirs := h.ScanDirs
		if dirs == nil {
			dirs = []string{os.TempDir(), "/tmp", "/var/tmp", "/dev/shm"}
		}
		for _, f := range ownedFiles(dedupe(dirs), uid, root) {
			rep.Leaks = append(rep.Leaks, Leak{LeakFile, f})
		}
	}
	kinds := map[string]LeakKind{"container": LeakContainer, "network": LeakNetwork, "volume": LeakVolume}
	for _, kind := range Kinds {
		found, err := h.runtime().List(ctx, kind, attempt)
		if err != nil {
			rep.Leaks = append(rep.Leaks, Leak{LeakUnverified, fmt.Sprintf("%s listing: %s", kind, firstLine(err.Error()))})
			continue
		}
		for _, r := range found {
			rep.Leaks = append(rep.Leaks, Leak{kinds[string(kind)], r.ID})
		}
	}
	if rep.Clean() {
		return rep, nil
	}
	return rep, leakError(ReasonLeak, "%s", rep.Reason())
}

// checkProcs adds the processes and listening sockets that survived.
func (h *Host) checkProcs(rep *Report, attempt string, uid int) {
	procs, err := scanProcs(attempt, uid)
	if err != nil {
		rep.Skipped = append(rep.Skipped, "processes: "+err.Error())
		rep.Skipped = append(rep.Skipped, "sockets: "+err.Error())
		return
	}
	inodes := map[uint64]bool{}
	for _, p := range procs {
		rep.Leaks = append(rep.Leaks, Leak{LeakProcess, fmt.Sprintf("pid %d (%s) uid %d pgid %d", p.PID, p.Cmd, p.UID, p.PGID)})
		for n := range p.Inodes {
			inodes[n] = true
		}
	}
	socks, err := listeners()
	if err != nil {
		rep.Leaks = append(rep.Leaks, Leak{LeakUnverified, "listening sockets: " + firstLine(err.Error())})
		return
	}
	for _, l := range socks {
		if (uid > 0 && l.UID == uid) || inodes[l.Inode] {
			rep.Leaks = append(rep.Leaks, Leak{LeakSocket, fmt.Sprintf("tcp listener port %d uid %d", l.Port, l.UID)})
		}
	}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range in {
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
