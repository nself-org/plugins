package contention

// Purpose: cgroup v2 limits for leased jobs: one parent group under the agent's own cgroup holds the
// operator's reservation (cpu.max, memory.max), one leaf per attempt lets a job join it.
// Inputs:  the reservation Share, an attempt id.
// Outputs: the parent and leaf directories, or a reason the limits could not be applied.
// Constraints: no root: the parent is created only when the agent's cgroup is delegated (writable with
//          the cpu controller enabled); otherwise the caller reports "unknown" with the reason. Group
//          names come from the attempt id after the same safe-id check the executor applies.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

const (
	parentGroup = "nself-ci-jobs"
	cpuPeriod   = 100000
)

var groupName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ownCgroup is the absolute directory of this process's cgroup (v2 only).
func (p Platform) ownCgroup() (string, error) {
	f, err := os.Open(p.SelfCgroup)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "0::"); ok {
			dir := filepath.Join(p.CgroupRoot, filepath.Clean("/"+rest))
			if !p.has(filepath.Join(p.CgroupRoot, "cgroup.controllers")) {
				return "", errors.New("cgroup v2 is not mounted")
			}
			return dir, nil
		}
	}
	return "", errors.New("no cgroup v2 entry for this process")
}

// jobParent creates the parent group holding the reservation and returns its directory.
func (p Platform) jobParent(sh reservation.Share) (string, error) {
	own, err := p.ownCgroup()
	if err != nil {
		return "", err
	}
	// Best effort: a delegated cgroup lets us enable controllers for children; failure shows below.
	_ = os.WriteFile(filepath.Join(own, "cgroup.subtree_control"), []byte("+cpu +memory"), 0o644)
	dir := filepath.Join(own, parentGroup)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cgroup not writable: %w", err)
	}
	if sh.CPUCapped {
		quota := max(int64(sh.CPU*cpuPeriod), 1000)
		if err := os.WriteFile(filepath.Join(dir, "cpu.max"), fmt.Appendf(nil, "%d %d", quota, cpuPeriod), 0o644); err != nil {
			return "", fmt.Errorf("cpu.max not writable: %w", err)
		}
	}
	if sh.MemCapped && sh.MemMB > 0 {
		if err := os.WriteFile(filepath.Join(dir, "memory.max"), fmt.Appendf(nil, "%d", sh.MemMB*1024*1024), 0o644); err != nil {
			return "", fmt.Errorf("memory.max not writable: %w", err)
		}
	}
	return dir, nil
}

// leaf creates the attempt's group under parent and returns its directory.
func leaf(parent, attempt string) (string, error) {
	if !groupName.MatchString(attempt) {
		return "", errors.New("attempt id is not a safe group name")
	}
	dir := filepath.Join(parent, attempt)
	return dir, os.MkdirAll(dir, 0o755)
}

// CgroupUsageUsec is the cumulative CPU time (usage_usec) the parent group's jobs have used; ok is
// false when the file is absent. It is the measurement behind scenario 11.
func (p Platform) CgroupUsageUsec() (usec int64, ok bool) {
	own, err := p.ownCgroup()
	if err != nil {
		return 0, false
	}
	b, err := os.ReadFile(filepath.Join(own, parentGroup, "cpu.stat"))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, found := strings.CutPrefix(line, "usage_usec "); found {
			_, err := fmt.Sscan(v, &usec)
			return usec, err == nil
		}
	}
	return 0, false
}

// Sweep removes empty attempt groups left by jobs that did not exit cleanly. rmdir fails on a group
// that still holds processes, which is the right answer.
func (p Platform) Sweep() {
	own, err := p.ownCgroup()
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(filepath.Join(own, parentGroup))
	for _, e := range entries {
		if e.IsDir() {
			_ = os.Remove(filepath.Join(own, parentGroup, e.Name()))
		}
	}
}

// ReleaseGroup removes one attempt's group.
func (p Platform) ReleaseGroup(attempt string) {
	if own, err := p.ownCgroup(); err == nil && groupName.MatchString(attempt) {
		_ = os.Remove(filepath.Join(own, parentGroup, attempt))
	}
}
