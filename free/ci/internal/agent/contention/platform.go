package contention

// Purpose: the host facts the priority layer needs (OS, tool paths, cgroup mount), behind one struct a
// test can fill with a temp directory.
// Inputs:  the running OS.
// Outputs: Platform.
// Constraints: tools are found by absolute path with os.Stat, never through PATH or os/exec; the
//          argv prefix the isolator builds uses those absolute paths.

import (
	"os"
	"runtime"
)

// Platform names where the OS pieces live.
type Platform struct {
	GOOS string
	// CgroupRoot is the cgroup v2 mount; SelfCgroup is /proc/self/cgroup.
	CgroupRoot, SelfCgroup string
	// Has reports whether a file exists; nil uses os.Stat.
	Has func(path string) bool
}

// HostPlatform is the running machine.
func HostPlatform() Platform {
	return Platform{GOOS: runtime.GOOS, CgroupRoot: "/sys/fs/cgroup", SelfCgroup: "/proc/self/cgroup"}
}

func (p Platform) has(path string) bool {
	if p.Has != nil {
		return p.Has(path)
	}
	_, err := os.Stat(path)
	return err == nil
}

// first returns the first existing path of paths, or "".
func (p Platform) first(paths ...string) string {
	for _, c := range paths {
		if p.has(c) {
			return c
		}
	}
	return ""
}
