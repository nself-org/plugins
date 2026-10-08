//go:build linux

package exec

import (
	"bytes"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var subreaperOnce sync.Once
var subreaperErr error

func ensureSubreaper() error {
	subreaperOnce.Do(func() {
		_, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, 36, 1, 0, 0, 0, 0)
		if errno != 0 {
			subreaperErr = coded("E610", "subreaper setup failed: "+errno.Error())
		}
	})
	return subreaperErr
}

func reapDescendants(pids []int) {
	deadline := time.Now().Add(200 * time.Millisecond)
	for {
		pending := false
		for _, pid := range pids {
			var status syscall.WaitStatus
			_, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
			if err == nil && processExists(pid) {
				pending = true
			}
		}
		if !pending || time.Now().After(deadline) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func processExists(pid int) bool {
	return syscall.Kill(pid, 0) == nil || os.IsPermission(syscall.Kill(pid, 0))
}

func groupAlive(pgid int) bool {
	out, err := osexec.Command("ps", "-A", "-o", "pgid=", "-o", "stat=").Output()
	if err != nil {
		return syscall.Kill(-pgid, 0) == nil
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		group, parseErr := strconv.Atoi(fields[0])
		if parseErr == nil && group == pgid && fields[1][0] != 'Z' {
			return true
		}
	}
	return false
}

func adoptedDescendants(parents map[int]int, attempt string) []int {
	if attempt == "" {
		return nil
	}
	tag := []byte("NSELF_CI_EXEC_ATTEMPT=" + attempt + "\x00")
	var adopted []int
	for pid, ppid := range parents {
		if ppid != os.Getpid() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
		if err == nil && bytes.Contains(data, tag) {
			adopted = append(adopted, pid)
		}
	}
	return adopted
}
