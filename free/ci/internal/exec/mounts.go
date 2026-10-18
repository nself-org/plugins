package exec

// Purpose: host paths a job may see (cache restore), with read-only and
// read-write modes.
// Inputs:  JobSpec.Mounts. Outputs: docker --mount arguments, or a refusal.
// Constraints: a container gets a bind mount (read-only is enforced by
// Docker); a process job already sees the host path, cannot be held
// read-only, and so refuses a read-only mount instead of ignoring the flag.

import (
	"path"
	"strings"
)

// Mount is a host path made visible to a job at Target (container jobs).
type Mount struct {
	Host, Target string
	ReadOnly     bool
}

var reservedTargets = []string{"/repo", "/proc", "/sys", "/dev"}

func checkMounts(s JobSpec) error {
	seen := map[string]bool{}
	for _, m := range s.Mounts {
		for _, p := range []string{m.Host, m.Target} {
			if !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, ",\r\n\x00") {
				return coded("E612", "mount paths must be absolute, clean and free of commas and newlines")
			}
		}
		if strings.HasSuffix(m.Host, "docker.sock") {
			return coded("E612", "mounting a container runtime socket is refused")
		}
		if m.Target == "/" || seen[m.Target] {
			return coded("E612", "mount target is the root or repeated")
		}
		seen[m.Target] = true
		for _, r := range reservedTargets {
			if m.Target == r || strings.HasPrefix(m.Target, r+"/") {
				return coded("E612", "mount target collides with a reserved container path")
			}
		}
		if m.ReadOnly && s.Job.Isolation != "container" {
			return coded("E611", "a read-only mount needs container isolation")
		}
	}
	return nil
}

func mountArgs(ms []Mount) []string {
	var args []string
	for _, m := range ms {
		spec := "type=bind,src=" + m.Host + ",dst=" + m.Target
		if m.ReadOnly {
			spec += ",readonly"
		}
		args = append(args, "--mount", spec)
	}
	return args
}
