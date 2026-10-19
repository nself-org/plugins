//go:build darwin

package exec

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// processTable returns pid -> parent pid for every process, read with one
// sysctl (no fork, no cgo).
func processTable() (map[int]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	parents := make(map[int]int, len(procs))
	for i := range procs {
		parents[int(procs[i].Proc.P_pid)] = int(procs[i].Eproc.Ppid)
	}
	return parents, nil
}

// nativeStartID names the start instant of pid: it changes when the pid is reused.
func nativeStartID(pid int) (string, bool) {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || int(p.Proc.P_pid) != pid {
		return "", false
	}
	tv := p.Proc.P_starttime
	return "st:" + strconv.FormatInt(int64(tv.Sec), 10) + "." + strconv.FormatInt(int64(tv.Usec), 10), true
}
