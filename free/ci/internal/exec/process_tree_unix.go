//go:build !windows

package exec

import (
	osexec "os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// processTree remembers children even after a shell exits or a child calls setsid.
type processTree struct {
	root    int
	attempt string
	mu      sync.Mutex
	seen    map[int]struct{}
	done    chan struct{}
	stopped chan struct{}
}

func trackDescendants(root int, attempt string) *processTree {
	t := &processTree{root: root, attempt: attempt, seen: make(map[int]struct{}), done: make(chan struct{}), stopped: make(chan struct{})}
	t.snapshot()
	go func() {
		defer close(t.stopped)
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				t.snapshot()
			case <-t.done:
				return
			}
		}
	}()
	return t
}

func (t *processTree) stop() {
	close(t.done)
	<-t.stopped
	t.signalKill()
	reapDescendants(t.pids())
}

// snapshot reads the process table and adds every descendant of the root (and
// every adopted orphan) to the seen set. A pass that found new processes is
// repeated (at most three passes) so forks made while the table was being read
// are caught. The table comes from /proc or sysctl, which cost no fork; ps is
// only the fallback when those fail.
func (t *processTree) snapshot() {
	for i := 0; i < 3; i++ {
		parents, err := processTable()
		if err != nil {
			if parents, err = psProcessTable(); err != nil {
				return
			}
		}
		if !t.absorb(parents) {
			return
		}
	}
}

// absorb adds the descendants found in parents to the seen set and reports
// whether it added any.
func (t *processTree) absorb(parents map[int]int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	known := map[int]struct{}{t.root: {}}
	for pid := range t.seen {
		known[pid] = struct{}{}
	}
	changed := false
	for _, pid := range adoptedDescendants(parents, t.attempt) {
		known[pid] = struct{}{}
		if _, ok := t.seen[pid]; !ok {
			t.seen[pid] = struct{}{}
			changed = true
		}
	}
	for {
		added := false
		for pid, ppid := range parents {
			if _, ok := known[ppid]; !ok {
				continue
			}
			if _, ok := known[pid]; ok {
				continue
			}
			known[pid] = struct{}{}
			t.seen[pid] = struct{}{}
			added = true
			changed = true
		}
		if !added {
			return changed
		}
	}
}

// psProcessTable is the fork-based fallback for hosts where the native table
// is unreadable.
func psProcessTable() (map[int]int, error) {
	out, err := osexec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=").Output()
	if err != nil {
		return nil, err
	}
	parents := make(map[int]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, e1 := strconv.Atoi(fields[0])
		ppid, e2 := strconv.Atoi(fields[1])
		if e1 == nil && e2 == nil {
			parents[pid] = ppid
		}
	}
	return parents, nil
}

func (t *processTree) pids() []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	pids := make([]int, 0, len(t.seen))
	for pid := range t.seen {
		pids = append(pids, pid)
	}
	return pids
}

func (t *processTree) signalTerminate() { t.signal(syscall.SIGTERM) }
func (t *processTree) signalKill()      { t.signal(syscall.SIGKILL) }
func (t *processTree) anyAlive() bool {
	for _, pid := range t.pids() {
		if syscall.Kill(pid, 0) == nil {
			return true
		}
	}
	return false
}
func (t *processTree) signal(sig syscall.Signal) {
	for _, pid := range t.pids() {
		_ = syscall.Kill(pid, sig)
	}
}
