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

func unknownFact[T any](src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Source: src, ObservedAt: now, Confidence: "unknown"}
}

func knownFact[T any](v T, src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

func draftCapability(id string, r enroll.Request, now time.Time) model.Capability {
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity = model.CapabilityIdentity{
		ID: id, Name: id, Provider: "agent", Transport: "agent", Labels: []string{}, HostKeyFingerprints: []string{},
		Ownership: unknownFact[string]("assigned", now), MachineID: unknownFact[string]("self", now), AgeRecipient: unknownFact[string]("self", now),
	}
	if r.MachineID != "" {
		c.Identity.MachineID = knownFact(r.MachineID, "self", now)
	}
	c.Platform = model.CapabilityPlatform{
		OS: knownFact(r.OS, "self", now), Arch: knownFact(r.Arch, "self", now), Kernel: unknownFact[string]("self", now),
		OSVersion: unknownFact[string]("self", now), Virtualization: unknownFact[string]("self", now),
	}
	c.Resources = model.CapabilityResources{
		CPU: unknownFact[float64]("self", now), MemMB: unknownFact[int64]("self", now), DiskFreeMB: unknownFact[int64]("self", now),
		Accelerators: unknownFact[[]string]("self", now),
		Reserved:     model.CapabilityReserved{CPU: unknownFact[float64]("assigned", now), MemMB: unknownFact[int64]("assigned", now)},
	}
	c.Tools = model.CapabilityTools{
		Docker: unknownFact[bool]("self", now), Podman: unknownFact[bool]("self", now), GVisor: unknownFact[bool]("self", now), Tart: unknownFact[bool]("self", now),
		Toolchains: map[string]model.Fact[string]{}, Browsers: unknownFact[[]string]("self", now), Services: unknownFact[[]string]("self", now),
		Custom: map[string]model.Fact[string]{},
	}
	c.Availability = model.CapabilityAvailability{
		State: knownFact("offline", "assigned", now), Slots: unknownFact[int]("self", now), Running: unknownFact[int]("self", now),
		BatteryPct: unknownFact[int]("self", now), PluggedIn: unknownFact[bool]("self", now), Interactive: unknownFact[string]("self", now),
		Persistence: unknownFact[bool]("assigned", now), OnBattery: unknownFact[string]("assigned", now), InteractivePolicy: unknownFact[string]("assigned", now),
	}
	c.Location = model.CapabilityLocation{Kind: unknownFact[string]("self", now), Region: unknownFact[string]("self", now)}
	c.Economics = model.CapabilityEconomics{
		Class: unknownFact[string]("self", now), MarginalCostPerMin: unknownFact[float64]("self", now),
		Allowance: model.CapabilityAllowance{KnownRemaining: unknownFact[float64]("self", now), ResetAt: unknownFact[time.Time]("self", now), Confidence: unknownFact[string]("self", now)},
	}
	c.Separation = model.CapabilitySeparation{UIDSeparation: unknownFact[bool]("self", now), UserNS: unknownFact[bool]("self", now), SigningEligible: unknownFact[bool]("assigned", now)}
	c.Verified = model.CapabilityVerified{By: "self", At: now}
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1, AgentVersion: r.AgentVersion}
	c.Lifecycle = model.CapabilityLifecycle{Discovered: true, AuthorizedProjects: []string{}}
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
		Accepts: knownFact(accepts, "assigned", now), SecretClasses: knownFact(secrets, "assigned", now),
		Isolation: unknownFact[model.Isolation]("assigned", now), Network: unknownFact[[]model.NetworkScope]("assigned", now),
		PrivacyZone: unknownFact[model.PrivacyZone]("assigned", now),
	}
	if t.PrivacyZone != "" {
		out.PrivacyZone = knownFact(model.PrivacyZone(t.PrivacyZone), "assigned", now)
	}
	return out
}
