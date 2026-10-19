//go:build linux

package exec

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// tableWorkers bounds the goroutines of one table read.
const tableWorkers = 8

// procStat is the part of /proc/<pid>/stat the executor needs.
type procStat struct {
	state      byte
	ppid, pgrp int
	startTicks string
}

// readProcStat parses /proc/<pid>/stat. The command name may hold spaces and
// parentheses, so fields are counted from the last ')'.
func readProcStat(pid int) (procStat, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return procStat{}, false
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return procStat{}, false
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 20 || f[0] == "" {
		return procStat{}, false
	}
	ppid, e1 := strconv.Atoi(f[1])
	pgrp, e2 := strconv.Atoi(f[2])
	if e1 != nil || e2 != nil {
		return procStat{}, false
	}
	return procStat{state: f[0][0], ppid: ppid, pgrp: pgrp, startTicks: f[19]}, true
}

// procPIDs lists the numeric entries of /proc.
func procPIDs() ([]int, error) {
	dir, err := os.Open("/proc")
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	pids := make([]int, 0, len(names))
	for _, n := range names {
		if pid, err := strconv.Atoi(n); err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// processTable returns pid -> parent pid for every process, read from /proc
// without forking. The stat files are read on several goroutines: the tracker
// takes a snapshot when a job starts and every 50 ms after, and a host with a
// few hundred processes made each one a serial walk (D-1008).
func processTable() (map[int]int, error) {
	pids, err := procPIDs()
	if err != nil {
		return nil, err
	}
	ppids := make([]int, len(pids))
	readOK := make([]bool, len(pids)) // false: the process exited while the table was read
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(tableWorkers, len(pids)) {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= len(pids) {
					return
				}
				if st, ok := readProcStat(pids[i]); ok {
					ppids[i], readOK[i] = st.ppid, true
				}
			}
		})
	}
	wg.Wait()
	parents := make(map[int]int, len(pids))
	for i, pid := range pids {
		if readOK[i] {
			parents[pid] = ppids[i]
		}
	}
	return parents, nil
}

// nativeStartID names the start instant of pid: it changes when the pid is reused.
func nativeStartID(pid int) (string, bool) {
	st, ok := readProcStat(pid)
	if !ok {
		return "", false
	}
	return "st:" + st.startTicks + bootSuffix(), true
}

var (
	bootOnce sync.Once
	bootID   string
)

// bootSuffix ties start ticks (counted from boot) to one boot, so a marker kept
// across a reboot cannot match a new process that has the same pid and ticks.
func bootSuffix() string {
	bootOnce.Do(func() {
		if b, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
			bootID = "@" + strings.TrimSpace(string(b))
		}
	})
	return bootID
}

// groupAlive reports whether any non-zombie process is in group pgid.
func groupAlive(pgid int) bool {
	pids, err := procPIDs()
	if err != nil {
		return processGroupProbe(pgid)
	}
	for _, pid := range pids {
		if st, ok := readProcStat(pid); ok && st.pgrp == pgid && st.state != 'Z' {
			return true
		}
	}
	return false
}
