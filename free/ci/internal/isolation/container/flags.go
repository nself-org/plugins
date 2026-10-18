// Package container is the hardened container isolation profile (ADR 0025 level
// `container`, Epic D8).
package container

// Purpose: build the hardened `run` argv for one job container, purely.
// Inputs:  an exec.ContainerRun plus the profile's runtime facts and network plan.
// Outputs: an exec.ContainerCommand; nothing is started here (exec starts it).
// Constraints: never --privileged, a runtime socket, --pid/--ipc/--network=host, or a
// host path beyond the attempt workspace; the user is 65532 + slot, never the agent or
// coordinator UID; every container carries the attempt, role and coordinator labels.

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/isolation"
)

const (
	// BaseUID is the first job UID; a slot adds its index (Epic D12).
	BaseUID = 65532
	// DefaultPidsLimit caps processes per container.
	DefaultPidsLimit = 512
	// RoleLabel and the others are the labels SCHED-09 and the hygiene sweep list by.
	RoleLabel        = "role=ci-ephemeral"
	AttemptLabel     = "nself.ci.attempt"
	CoordinatorLabel = "coordinator_id"
	// LegacyCoordinatorLabel is the label the executor's orphan sweep reads.
	LegacyCoordinatorLabel = "nself.ci.coordinator"
)

// Params is everything Build needs beyond the run itself.
type Params struct {
	Run exec.ContainerRun
	// Binary is docker or podman.
	Binary string
	// ForbiddenUIDs are host UIDs the job must never run as (agent, coordinator).
	ForbiddenUIDs []int
	PidsLimit     int
	Network       isolation.NetworkPlan
	// Runtime selects an OCI runtime by name (the sandboxed profile sets runsc); empty keeps the default.
	Runtime string
	// UsernsAuto adds --userns=auto (rootful podman gives each container its own mapping).
	UsernsAuto bool
	// Entrypoint overrides the image entrypoint (the cleanup helper uses sh).
	Entrypoint string
	// MaxUID, when positive, is the highest UID the runtime maps (rootless: MaxRootlessUID).
	MaxUID int
}

// MaxRootlessUID is the highest container UID a default rootless mapping (65536 subuids) holds.
const MaxRootlessUID = 65535

// JobUID returns the UID of slot: the slot-th UID from BaseUID that is not forbidden,
// so two slots never share a UID and no slot gets a forbidden one.
func JobUID(slot int, forbidden []int) (int, error) {
	if slot < 0 {
		return 0, fmt.Errorf("negative slot %d", slot)
	}
	skip := map[int]bool{0: true}
	for _, f := range forbidden {
		skip[f] = true
	}
	uid := BaseUID
	for n := slot; ; uid++ {
		if skip[uid] {
			continue
		}
		if n == 0 {
			return uid, nil
		}
		n--
	}
}

// SlotUID is JobUID with the runtime's UID ceiling (max <= 0 means none) applied: a slot whose UID
// the runtime cannot map is refused with E678 rather than failing at container start.
func SlotUID(slot int, forbidden []int, max int) (int, error) {
	n, err := JobUID(slot, forbidden)
	if err != nil {
		return 0, err
	}
	if max > 0 && n > max {
		return 0, isolation.Unavailable(isolation.ReasonSlotUID, "slot %d needs uid %d, above the %d this runtime maps", slot, n, max)
	}
	return n, nil
}

// Build returns the hardened run command for p.
func Build(p Params) (exec.ContainerCommand, error) {
	r := p.Run
	switch {
	case p.Binary != "docker" && p.Binary != "podman":
		return exec.ContainerCommand{}, fmt.Errorf("container runtime %q is not docker or podman", p.Binary)
	case r.Name == "" || r.Image == "" || len(r.Command) == 0:
		return exec.ContainerCommand{}, fmt.Errorf("container name, image and command are required")
	case !filepath.IsAbs(r.Workspace) || strings.ContainsAny(r.Workspace, ",\"\n") || strings.Contains(r.Workspace, ".sock"):
		return exec.ContainerCommand{}, fmt.Errorf("workspace %q is not a plain absolute path", r.Workspace)
	}
	pids := p.PidsLimit
	if pids <= 0 {
		pids = DefaultPidsLimit
	}
	n, err := SlotUID(r.Slot, p.ForbiddenUIDs, p.MaxUID)
	if err != nil {
		return exec.ContainerCommand{}, err
	}
	uid := strconv.Itoa(n)
	mem := strconv.Itoa(r.MemoryMB) + "m"
	args := []string{"run", "--rm", "--name", r.Name,
		"--label", AttemptLabel + "=" + r.AttemptID, "--label", RoleLabel,
		"--label", CoordinatorLabel + "=" + r.CoordinatorID, "--label", LegacyCoordinatorLabel + "=" + r.CoordinatorID,
		"--network=" + p.Network.Network}
	for _, dns := range p.Network.DNS {
		args = append(args, "--dns", dns)
	}
	args = append(args, "--cap-drop=ALL", "--security-opt", "no-new-privileges", "--read-only",
		"--tmpfs", "/tmp:rw,nosuid,size=256m", "--pids-limit", strconv.Itoa(pids),
		"--cpus="+strconv.FormatFloat(r.CPU, 'f', -1, 64), "--memory="+mem, "--memory-swap="+mem,
		"--user", uid+":"+uid, "--env", "HOME=/tmp", "--env", "TMPDIR=/tmp")
	if p.Runtime != "" {
		args = append(args, "--runtime", p.Runtime)
	}
	if p.UsernsAuto {
		args = append(args, "--userns=auto")
	}
	if p.Entrypoint != "" {
		args = append(args, "--entrypoint", p.Entrypoint)
	}
	for _, m := range r.Mounts {
		args = append(args, "--mount", m.Spec())
	}
	args = append(args, "--mount", "type=bind,src="+r.Workspace+",dst=/repo", "--workdir", r.Workdir)
	if r.EnvFile != "" {
		args = append(args, "--env-file", r.EnvFile)
	}
	args = append(args, r.Image)
	args = append(args, r.Command...)
	return exec.ContainerCommand{Binary: p.Binary, Args: args}, nil
}
