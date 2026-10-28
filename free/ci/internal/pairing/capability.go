package pairing

// Purpose: the capability document of a paired (pull) node and its trust mapping.
// Inputs:  the enrollment request (name, os, arch, machine id, agent version) and the approver's trust.
// Outputs: a model.Capability that passes model.ValidateCapability, with unknown facts explicit.
// Constraints: the node supplies only identity facts; trust, ownership and authorization are the
//          approver's (registry.SetTrust and Authorize), never the node's. This duplicates the draft
//          shape of internal/nodes/lifecycle (unexported there); debt row D-pairing-capability-draft
//          tracks moving it into model.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pairing/enroll"
)

func draftCapability(id string, r enroll.Request, now time.Time) model.Capability {
	c := model.NewCapability("self", "self", now)
	c.Identity.ID, c.Identity.Name = id, id
	c.Identity.Provider, c.Identity.Transport = "agent", "agent"
	if r.MachineID != "" {
		c.Identity.MachineID = model.KnownFact(r.MachineID, "self", now)
	}
	c.Platform.OS, c.Platform.Arch = model.KnownFact(r.OS, "self", now), model.KnownFact(r.Arch, "self", now)
	c.Availability.State = model.KnownFact("offline", "assigned", now)
	c.Verified = model.CapabilityVerified{By: "self", At: now}
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1, AgentVersion: r.AgentVersion}
	c.Lifecycle.Discovered = true
	return c
}

// trustOf maps the approver's fields to the capability trust document.
func trustOf(t TrustFields, now time.Time) model.CapabilityTrust {
	accepts := make([]model.TrustClass, len(t.Accepts))
	for i, a := range t.Accepts {
		accepts[i] = model.TrustClass(a)
	}
	secrets := make([]model.SecretClass, len(t.SecretClasses))
	for i, s := range t.SecretClasses {
		secrets[i] = model.SecretClass(s)
	}
	out := model.CapabilityTrust{
		Accepts: model.KnownFact(accepts, "assigned", now), SecretClasses: model.KnownFact(secrets, "assigned", now),
		Isolation: model.UnknownFact[model.Isolation]("assigned", now), Network: model.UnknownFact[[]model.NetworkScope]("assigned", now),
		PrivacyZone: model.UnknownFact[model.PrivacyZone]("assigned", now),
	}
	if t.PrivacyZone != "" {
		out.PrivacyZone = model.KnownFact(model.PrivacyZone(t.PrivacyZone), "assigned", now)
	}
	return out
}
