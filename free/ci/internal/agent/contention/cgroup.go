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
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/nodes/reservation"
)

const (
	parentGroup = "nself-ci-jobs"
	selfGroup   = "nself-ci-agent"
	cpuPeriod   = 100000
)

var groupName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ownCgroup is the absolute directory of the cgroup the agent runs in (v2 only); after delegate moved
// the process into its own child group, that group's parent is returned.
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
			if filepath.Base(dir) == selfGroup { // we stepped into our own child group earlier (delegate)
				dir = filepath.Dir(dir)
			}
			return dir, nil
		}
	}
	return "", errors.New("no cgroup v2 entry for this process")
}

// jobParent creates the parent group holding the reservation. A resource the operator did not reserve
// is set back to "max", so an earlier, larger reservation never lingers. cpuErr and memErr say which
// limit could not be written; err is set when no parent exists at all.
func (p Platform) jobParent(sh reservation.Share) (dir string, cpuErr, memErr, err error) {
	own, err := p.ownCgroup()
	if err != nil {
		return "", nil, nil, err
	}
	p.delegate(own)
	dir = filepath.Join(own, parentGroup)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, nil, fmt.Errorf("cgroup not writable: %w", err)
	}
	cpu, mem := fmt.Sprintf("max %d", cpuPeriod), "max"
	if sh.CPUCapped {
		cpu = fmt.Sprintf("%d %d", max(int64(sh.CPU*cpuPeriod), 1000), cpuPeriod)
	}
	if sh.MemCapped && sh.MemMB > 0 {
		mem = fmt.Sprint(sh.MemMB * 1024 * 1024)
	}
	if e := os.WriteFile(filepath.Join(dir, "cpu.max"), []byte(cpu), 0o644); e != nil {
		cpuErr = fmt.Errorf("cpu.max not writable: %w", e)
	}
	if e := os.WriteFile(filepath.Join(dir, "memory.max"), []byte(mem), 0o644); e != nil {
		memErr = fmt.Errorf("memory.max not writable: %w", e)
	}
	return dir, cpuErr, memErr, nil
}

// delegate enables the cpu and memory controllers for the children of own. cgroup v2 forbids that
// while own holds processes (enabling it anyway can leave the hierarchy in an invalid state), so this
// process first steps into a child group of its own, the way container runtimes do, and enables the
// controllers only when nothing else is left in own. Failures are not errors here: the limit files
// that stay missing are reported per resource by the caller.
func (p Platform) delegate(own string) {
	self := filepath.Join(own, selfGroup)
	if os.MkdirAll(self, 0o755) != nil {
		return
	}
	if os.WriteFile(filepath.Join(self, "cgroup.procs"), []byte(strconv.Itoa(os.Getpid())), 0o644) != nil {
		return
	}
	if b, err := os.ReadFile(filepath.Join(own, "cgroup.procs")); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		return // another process shares the group: leave the hierarchy as the operator set it up
	}
	for _, c := range []string{"+cpu", "+memory"} {
		_ = os.WriteFile(filepath.Join(own, "cgroup.subtree_control"), []byte(c), 0o644)
	}
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

// Sweep removes attempt groups that hold no process and are older than minAge (a group just made for a
// job that has not joined yet is empty too). rmdir fails on a group that still holds processes, which
// is the right answer.
func (p Platform) Sweep(minAge time.Duration) {
	own, err := p.ownCgroup()
	if err != nil {
		return
	}
	entries, _ := os.ReadDir(filepath.Join(own, parentGroup))
	for _, e := range entries {
		if info, err := e.Info(); err == nil && e.IsDir() && time.Since(info.ModTime()) >= minAge {
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
