//go:build linux

package exec

// Purpose: the descendant tracker's process view without reading the whole
// system. A snapshot used to read /proc/<pid>/stat for every process on the host
// (a hosted runner has several hundred), twice or more per exec-funnel call; the
// tracker only needs the processes below the job's root and the orphans this
// process adopted as a subreaper.
// Inputs:  the pids to walk down from (the job root and every descendant seen).
// Outputs: a pid -> parent pid map holding the live descendants of those pids
//          and the direct children of this process, which is all absorb and
//          adoptedDescendants read.
// Constraints: reads /proc/<pid>/task/<tid>/children (CONFIG_PROC_CHILDREN);
// a kernel without that file falls back to the full table, so the guarantee
// is the same on every host. A process that exits mid-walk is skipped.

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// childrenOf lists the direct children of pid across all its threads (a fork
// made by any thread makes a child of the process). gone is true when the
// process has exited; ok is false when the kernel does not expose the file.
func childrenOf(pid int) (kids []int, gone, ok bool) {
	base := "/proc/" + strconv.Itoa(pid) + "/task"
	dir, err := os.Open(base)
	if err != nil {
		return nil, errors.Is(err, fs.ErrNotExist), errors.Is(err, fs.ErrNotExist)
	}
	tids, err := dir.Readdirnames(-1)
	_ = dir.Close()
	if err != nil {
		return nil, false, false
	}
	for _, tid := range tids {
		data, err := os.ReadFile(base + "/" + tid + "/children")
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				if _, serr := os.Stat(base + "/" + tid); serr != nil {
					continue // the thread exited between the listing and the read
				}
			}
			return nil, false, false // no children file here: the caller falls back
		}
		for _, field := range strings.Fields(string(data)) {
			if child, err := strconv.Atoi(field); err == nil && child > 0 {
				kids = append(kids, child)
			}
		}
	}
	return kids, false, true
}

// scopedTable is processTable limited to what the tracker reads: the live
// descendants of roots plus this process's own children.
func scopedTable(roots []int) (map[int]int, error) {
	self := os.Getpid()
	own, _, ok := childrenOf(self)
	if !ok {
		return processTable()
	}
	parents := make(map[int]int, len(own)+8)
	for _, child := range own {
		parents[child] = self
	}
	visited := make(map[int]struct{}, len(roots))
	queue := append([]int(nil), roots...)
	for len(queue) > 0 {
		pid := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if _, seen := visited[pid]; seen {
			continue
		}
		visited[pid] = struct{}{}
		kids, gone, ok := childrenOf(pid)
		switch {
		case gone:
			continue
		case !ok:
			return processTable()
		}
		for _, child := range kids {
			parents[child] = pid
			queue = append(queue, child)
		}
	}
	return parents, nil
}
