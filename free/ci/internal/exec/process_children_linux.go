//go:build linux

package exec

// Purpose: find the descendants of a finished command without walking the whole
// process table. The kernel lists the children of every task in
// /proc/<pid>/task/<tid>/children, so the processes a command left behind (its
// own children, and the orphans the coordinator adopted as subreaper) are found
// by following those lists from a few known pids, not by reading /proc/<pid>/stat
// of every process on the host (D-1008).
// Inputs:  the coordinator's own pid and the pids already seen below the root.
// Outputs: pid -> parent pid for the edges visited.
// Constraints: false means "use the full table walk": the kernel lacks the
// children files (CONFIG_PROC_CHILDREN) or a list could not be read for a reason
// other than the process having exited.

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// childrenFiles tells whether the kernel offers /proc/<pid>/task/<tid>/children; probed once.
var childrenFiles = sync.OnceValue(func() bool {
	_, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(syscall.Gettid()) + "/children")
	return err == nil
})

// childrenOf lists the children of every thread of pid. A process that is gone
// has none (ok); any other read error is reported as not ok.
func childrenOf(pid int) (kids []int, ok bool) {
	dir := "/proc/" + strconv.Itoa(pid) + "/task"
	tids, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH)
	}
	for _, tid := range tids {
		data, err := os.ReadFile(dir + "/" + tid.Name() + "/children")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
				continue // the thread exited while the list was read
			}
			return nil, false
		}
		for _, f := range strings.Fields(string(data)) {
			if k, err := strconv.Atoi(f); err == nil && k > 0 {
				kids = append(kids, k)
			}
		}
	}
	return kids, true
}

// childrenReads counts the pids whose children were listed (tests).
var childrenReads atomic.Int64

// quickTable builds pid -> parent pid for what a finished command can have left:
// the coordinator's own children (adopted orphans among them), and everything
// below those the attempt tag claims or that were seen below the root before.
func quickTable(self, root int, seen []int, attempt string) (map[int]int, bool) {
	if !childrenFiles() {
		return nil, false
	}
	parents := map[int]int{}
	first, ok := childrenOf(self)
	childrenReads.Add(1)
	if !ok {
		return nil, false
	}
	for _, k := range first {
		parents[k] = self
	}
	queue := append(append([]int(nil), seen...), root)
	queue = append(queue, adoptedDescendants(parents, attempt)...)
	visited := map[int]struct{}{}
	for len(queue) > 0 {
		pid := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if _, done := visited[pid]; done {
			continue
		}
		visited[pid] = struct{}{}
		kids, ok := childrenOf(pid)
		childrenReads.Add(1)
		if !ok {
			return nil, false
		}
		for _, k := range kids {
			parents[k] = pid
			queue = append(queue, k)
		}
	}
	return parents, true
}
