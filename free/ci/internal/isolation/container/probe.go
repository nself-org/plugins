package container

// Purpose: find out what docker or podman can deliver on this runner, as facts.
// Inputs:  the runtime CLI's `info` output through exec.RuntimeQuery (or a fake in tests).
// Outputs: an isolation.Availability and the runtime Facts the argv builder uses.
// Constraints: a probe only ever lowers the level; a separated node (agent user
// without docker group) accepts only a rootless runtime, because a rootful socket
// there is root-equivalent (Epic D8).

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

// Runner asks the container runtime read-only questions.
type Runner interface {
	Query(ctx context.Context, binary string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Query(ctx context.Context, binary string, args ...string) ([]byte, error) {
	return exec.RuntimeQuery(ctx, binary, args...)
}

// Facts describes the chosen runtime.
type Facts struct {
	Binary   string
	Rootless bool
	Userns   bool
	Runtimes []string
	Server   string
	// Limits is true when the runtime enforces pids, memory and cpu limits (cgroup controllers present
	// and delegated). Without them docker and podman start the container and silently drop the flags.
	Limits bool
}

type dockerInfo struct {
	ServerVersion   string                     `json:"ServerVersion"`
	SecurityOptions []string                   `json:"SecurityOptions"`
	Runtimes        map[string]json.RawMessage `json:"Runtimes"`
	MemoryLimit     bool                       `json:"MemoryLimit"`
	PidsLimit       bool                       `json:"PidsLimit"`
	CPUCfsQuota     bool                       `json:"CPUCfsQuota"`
}

type podmanInfo struct {
	Host struct {
		Security struct {
			Rootless bool `json:"rootless"`
		} `json:"security"`
		OCIRuntime struct {
			Name string `json:"name"`
		} `json:"ociRuntime"`
		CgroupVersion     string   `json:"cgroupVersion"`
		CgroupControllers []string `json:"cgroupControllers"`
	} `json:"host"`
	Version struct {
		Version string `json:"Version"`
	} `json:"version"`
}

// probeRuntime tries docker, then podman, and keeps the first runtime the node may use.
func probeRuntime(ctx context.Context, r Runner, separated bool) (Facts, isolation.Availability) {
	reason := isolation.ReasonRuntimeMissing
	rank := map[string]int{isolation.ReasonRuntimeMissing: 0, isolation.ReasonProbeFailed: 1, isolation.ReasonRootful: 2, isolation.ReasonLimits: 3}
	note := func(next string) {
		if rank[next] > rank[reason] {
			reason = next
		}
	}
	for _, binary := range []string{"docker", "podman"} {
		facts, err := inspectRuntime(ctx, r, binary)
		if err != nil {
			if !exec.IsRuntimeMissing(err) {
				note(isolation.ReasonProbeFailed) // installed but not answering: daemon down, permission denied, timeout
			}
			continue
		}
		if separated && !facts.Rootless {
			note(isolation.ReasonRootful)
			continue
		}
		if !facts.Limits {
			note(isolation.ReasonLimits) // a fork bomb or memory hog would not be stopped
			continue
		}
		return facts, isolation.Availability{Level: "container", Facts: facts.toMap()}
	}
	return Facts{}, isolation.Availability{Reason: reason}
}

func inspectRuntime(ctx context.Context, r Runner, binary string) (Facts, error) {
	out, err := r.Query(ctx, binary, "info", "--format", "{{json .}}")
	if err != nil {
		return Facts{}, err
	}
	f := Facts{Binary: binary}
	if binary == "podman" {
		var info podmanInfo
		if err := json.Unmarshal(out, &info); err != nil {
			return Facts{}, err
		}
		f.Rootless, f.Server = info.Host.Security.Rootless, info.Version.Version
		f.Limits = info.Host.CgroupVersion == "v2" && hasAll(info.Host.CgroupControllers, "cpu", "memory", "pids")
		if info.Host.OCIRuntime.Name != "" {
			f.Runtimes = []string{info.Host.OCIRuntime.Name}
		}
		return f, nil
	}
	var info dockerInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return Facts{}, err
	}
	f.Server = info.ServerVersion
	f.Limits = info.MemoryLimit && info.PidsLimit && info.CPUCfsQuota
	for _, opt := range info.SecurityOptions {
		switch {
		case strings.Contains(opt, "name=rootless"):
			f.Rootless = true
		case strings.Contains(opt, "name=userns"):
			f.Userns = true
		}
	}
	for name := range info.Runtimes {
		f.Runtimes = append(f.Runtimes, name)
	}
	sort.Strings(f.Runtimes)
	return f, nil
}

func hasAll(have []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			found = found || h == w
		}
		if !found {
			return false
		}
	}
	return true
}

func (f Facts) toMap() map[string]string {
	return map[string]string{"runtime": f.Binary, "rootless": strconv.FormatBool(f.Rootless), "userns": strconv.FormatBool(f.Userns),
		"limits": strconv.FormatBool(f.Limits), "runtimes": strings.Join(f.Runtimes, ","), "server_version": f.Server}
}
