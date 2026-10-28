package trust

// Purpose: the production boundary (Epic D21, D28; ADR 0024 section 9, ADR 0025 section 2): the rule that keeps CI
// work off deploy hosts and off an under-isolated coordinator host. It is appended to AdmissionRules, so
// Admissible returns its reasons with every other rule's.
// Inputs:  JobFacts and the Declaration facts DeployHost{Matched, Override}, CoordinatorHost, CoordinatorMode,
//          Isolation (all sourced by the caller, never from a node's own hello).
// Outputs: BoundaryDeployHost, BoundaryOverrideMinimum, BoundaryCoordinatorHost reasons; RequiresAudit.
// Constraints: pure (no os, net or database/sql); deploy-host matching belongs to P7-NODE-14, so only its facts
//          arrive here; an unknown trust class decodes to untrusted, an unknown isolation to rank -1
//          (below process) and an unknown coordinator mode to team.

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() { AdmissionRules = append(AdmissionRules, admitBoundary) }

// boundaryFloor is the isolation both the overridden deploy host and the coordinator host need.
const boundaryFloor model.Isolation = "vm"

// belowFloor is true when the declared isolation does not reach boundaryFloor; an unknown
// value ranks -1, so it counts as below.
func belowFloor(i model.Isolation) bool {
	return RankIsolation(i) < RankIsolation(boundaryFloor)
}

// ownTrust is true for the classes that may run on an overridden deploy host.
func ownTrust(c model.TrustClass) bool {
	t := DecodeTrust(string(c))
	return t == "owner" || t == "internal"
}

// admitBoundary applies D21 and D28.
func admitBoundary(job JobFacts, d Declaration, _ Effective) []Reason {
	var out []Reason
	if d.DeployHost.Matched {
		if !d.DeployHost.Override {
			out = append(out, BoundaryDeployHost)
		} else if !ownTrust(job.Trust) || belowFloor(d.Isolation) || job.Release || releaseSecret(job.SecretClasses) {
			out = append(out, BoundaryOverrideMinimum)
		}
	}
	if d.CoordinatorHost && coordinatorFloorApplies(job, d) && belowFloor(d.Isolation) {
		out = append(out, BoundaryCoordinatorHost)
	}
	return out
}

// coordinatorFloorApplies is D28: the vm floor binds in team mode, or when the job is someone else's work
// (collaborator or untrusted). A personal coordinator keeps its operator's owner and internal jobs at their
// normal minimums.
func coordinatorFloorApplies(job JobFacts, d Declaration) bool {
	if DecodeCoordinatorMode(string(d.CoordinatorMode)) == Team {
		return true
	}
	return !ownTrust(job.Trust)
}

// BoundaryMinIsolation is the level a job placed on d must run at under D21 and D28: the vm floor when
// the placement is on an overridden deploy host, or on a coordinator host where the floor binds, else "".
// Admission checks the node's declared level; a delivery layer that sets the job's own level uses this so
// the job is not run below the level admission relied on (P7-TRUST-18).
func BoundaryMinIsolation(job JobFacts, d Declaration) model.Isolation {
	if d.DeployHost.Matched && d.DeployHost.Override || d.CoordinatorHost && coordinatorFloorApplies(job, d) {
		return boundaryFloor
	}
	return ""
}

// RequiresAudit is true for every placement on an overridden deploy host: the placement is admitted only
// with an audit row, whatever the job.
func RequiresAudit(d Declaration) bool {
	return d.DeployHost.Matched && d.DeployHost.Override
}
