package update

// Purpose: the protocol ceiling of an upgrade (hq NODE D11, X19): a node never moves to a release that cannot speak to every
//          coordinator it is registered with.
// Inputs:  the agent's <state>/coordinators.json records (P7-NODE-23) and the verified manifest of the target release.
// Outputs: nil, or E666 reason ceiling naming the coordinator.
// Constraints: a record holds the protocol range the coordinator accepted from this agent; the release must overlap that range
//          for every coordinator (its protocol_min at most the coordinator's max, its protocol_max at least the coordinator's
//          min), and its version must not be below the coordinator's min_agent_version. No record means no ceiling.

import (
	"github.com/nself-org/plugins/free/ci/internal/agent/session"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
)

// Ceiling is the highest protocol version every recorded coordinator accepts (0 when none is recorded).
func Ceiling(records []session.CoordinatorRecord) int {
	ceil := 0
	for _, r := range records {
		if ceil == 0 || r.ProtocolMax < ceil {
			ceil = r.ProtocolMax
		}
	}
	return ceil
}

// CheckCeiling refuses a release that some recorded coordinator cannot talk to.
func CheckCeiling(records []session.CoordinatorRecord, m verify.Manifest) error {
	for _, r := range records {
		switch {
		case m.ProtocolMin > r.ProtocolMax:
			return refuse(ReasonCeiling, "version %s needs protocol %d or higher; coordinator %s accepts up to %d", m.Version, m.ProtocolMin, r.ID, r.ProtocolMax)
		case m.ProtocolMax < r.ProtocolMin:
			return refuse(ReasonCeiling, "version %s speaks protocol %d at most; coordinator %s needs %d or higher", m.Version, m.ProtocolMax, r.ID, r.ProtocolMin)
		case r.MinAgentVersion != "" && verify.ValidVersion(r.MinAgentVersion) && Compare(m.Version, r.MinAgentVersion) < 0:
			return refuse(ReasonCeiling, "version %s is below %s, the minimum agent version coordinator %s accepts", m.Version, r.MinAgentVersion, r.ID)
		}
	}
	return nil
}
