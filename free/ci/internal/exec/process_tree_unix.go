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

func (t *processTree) snapshot() {
	// ps is available on supported Unix hosts. Repeated walks capture forks that
	// occur while a prior snapshot is being read.
	for i := 0; i < 3; i++ {
		out, err := osexec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=").Output()
		if err != nil {
			return
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
		t.mu.Lock()
		known := map[int]struct{}{t.root: {}}
		for pid := range t.seen {
			known[pid] = struct{}{}
		}
		for _, pid := range adoptedDescendants(parents, t.attempt) {
			known[pid] = struct{}{}
			t.seen[pid] = struct{}{}
		}
		changed := false
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
				break
			}
		}
		t.mu.Unlock()
		if !changed {
			return
		}
	}
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
