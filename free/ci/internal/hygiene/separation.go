package hygiene

// Purpose: Separation, the policy that picks the host UID a job runs under. Hygiene supplies the
// policy; P7-NODE-06 creates the users and the sudoers rule, the agent runs the shim.
// Inputs:  the node's provisioning facts and the slot of the attempt.
// Outputs: a Plan (mode, UID, serial flag) or E682 when no separation exists and no exception applies.
// Constraints (Epic D12, invariant I17): never the agent's or the coordinator's UID outside the one
// exception (owner trust, process level, PERSONAL coordinator, the operator's own machine or
// non-separated operator node); never the same UID for two slots in a separated mode; a node with
// fewer job users than slots serves one process-level attempt at a time (isolation.slot_uid_shared).

import (
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Mode is how a job is kept off the agent's UID.
type Mode string

// The separation modes (Epic D12).
const (
	ContainerMapped Mode = "container-mapped"
	JobShim         Mode = "job-shim"
	Userns          Mode = "userns"
	OwnerException  Mode = "owner-exception"
)

// Facts are what Separation decides from.
type Facts struct {
	Isolation   model.Isolation
	Trust       model.TrustClass
	Coordinator trust.CoordinatorMode
	// Ownership is "operator" for the operator's own runner.
	Ownership string
	// Slots is the number of execution slots on the node.
	Slots int
	// AgentUID and CoordinatorUID are never handed to a job outside the exception.
	AgentUID, CoordinatorUID int
	// JobShim is true when the agent's job-shim sudo rule exists (system install).
	JobShim bool
	// JobUIDs are the host UIDs of nself-ci-job-0..n-1, index = slot.
	JobUIDs []int
	// Userns is true when ProbeUserns passed; SubUIDStart/Count are the agent user's subordinate range.
	Userns                   bool
	SubUIDStart, SubUIDCount int
	// OperatorNode is true for the operator's own machine or non-separated operator node.
	OperatorNode bool
}

// Plan is the chosen separation for one slot.
type Plan struct {
	Mode Mode
	UID  int
	// User is the account name for job-shim.
	User string
	// Serial is true when the node must run process-level attempts one at a time.
	Serial bool
	// Reason is a stable trust reason when the plan carries a restriction.
	Reason trust.Reason
}

// JobUserName is the system account of a slot (Epic D12; P7-NODE-06 creates it).
func JobUserName(slot int) string { return fmt.Sprintf("nself-ci-job-%d", slot) }

// Separation picks the separation for the attempt in slot.
func Separation(f Facts, slot int) (Plan, error) {
	if slot < 0 || (f.Slots > 0 && slot >= f.Slots) {
		return Plan{}, noUIDError(ReasonNoJobUID, "slot %d is outside the node's %d slots", slot, f.Slots)
	}
	forbidden := []int{f.AgentUID, f.CoordinatorUID}
	if containerLevel(f.Isolation) {
		uid, err := container.JobUID(slot, forbidden)
		if err != nil {
			return Plan{}, noUIDError(ReasonNoJobUID, "%v", err)
		}
		return Plan{Mode: ContainerMapped, UID: uid}, nil
	}
	if f.JobShim && len(f.JobUIDs) > 0 {
		return jobShim(f, slot, forbidden)
	}
	if f.Userns && f.SubUIDStart > 0 && f.SubUIDCount > slot {
		uid := f.SubUIDStart + slot
		if contains(forbidden, uid) {
			return Plan{}, noUIDError(ReasonNoJobUID, "subordinate uid %d is the agent's or coordinator's", uid)
		}
		return Plan{Mode: Userns, UID: uid}, nil
	}
	if ownerException(f) {
		// no separation exists: the operator's own owner-trust work runs as the operator
		return Plan{Mode: OwnerException, UID: f.AgentUID, Serial: true, Reason: trust.IsolationSlotUIDShared}, nil
	}
	return Plan{}, noUIDError(ReasonNoSeparation, "no job UID separation exists and the owner exception does not apply")
}

func jobShim(f Facts, slot int, forbidden []int) (Plan, error) {
	seen := map[int]bool{}
	for _, u := range f.JobUIDs {
		if contains(forbidden, u) || u <= 0 || seen[u] {
			return Plan{}, noUIDError(ReasonNoJobUID, "job user uid %d is invalid, shared or the agent's", u)
		}
		seen[u] = true
	}
	if slot >= len(f.JobUIDs) {
		return Plan{}, noUIDError(ReasonNoJobUID, "no job user is provisioned for slot %d", slot)
	}
	plan := Plan{Mode: JobShim, UID: f.JobUIDs[slot], User: JobUserName(slot)}
	if f.Slots > len(f.JobUIDs) {
		plan.Serial, plan.Reason = true, trust.IsolationSlotUIDShared
	}
	return plan, nil
}

// containerLevel reports whether the level runs the job inside a container or VM, where the UID is
// mapped inside it (anything unknown is treated as process level, the strictest case).
func containerLevel(level model.Isolation) bool {
	switch level {
	case "container", "sandboxed-container", "vm", "ephemeral-vm", "hosted-disposable":
		return true
	}
	return false
}

// ownerException is the single I17 exception (ADR 0025 amendment, Epic D12).
func ownerException(f Facts) bool {
	return f.Trust == "owner" && f.Isolation == "process" && f.Coordinator == trust.Personal &&
		f.Ownership == "operator" && f.OperatorNode
}

func contains(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
