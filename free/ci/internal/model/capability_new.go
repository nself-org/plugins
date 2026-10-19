package model

// Purpose: the one scaffold of a capability document for a node the CLI registers by itself, so that
// the node builders (lifecycle.newCapability for `nodes add`, provision.RunnerCapability for a
// provisioned runner) share it and only set what they know (D-1233).
// Inputs:  the source name for facts a probe would observe, the source name for facts the node reports
//          about itself, and the observation time.
// Outputs: a Capability where every fact is unknown, lists and maps are empty (never nil), the protocol is v1,
//          the node is discovered and verified by selfSrc at now. It does not pass ValidateCapability
//          until the caller sets the identity, the platform os and the trust accepts.
// Constraints: facts that only the admin can assign (ownership, reserved, persistence, power policy,
//          trust) are unknown from source "assigned"; callers overwrite the facts they know.

import "time"

// UnknownFact is a fact nobody has observed yet.
func UnknownFact[T any](src string, now time.Time) Fact[T] {
	return Fact[T]{Source: src, ObservedAt: now, Confidence: "unknown"}
}

// KnownFact is a fact observed or assigned from src.
func KnownFact[T any](v T, src string, now time.Time) Fact[T] {
	return Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

// NewCapability returns the all-unknown scaffold. probeSrc names facts a probe observes, selfSrc facts
// the node reports about itself.
func NewCapability(probeSrc, selfSrc string, now time.Time) Capability {
	const assigned = "assigned"
	return Capability{
		Schema: "ci.runner-capability/v1",
		Identity: CapabilityIdentity{
			Provider: "ssh", Labels: []string{}, HostKeyFingerprints: []string{},
			Ownership: UnknownFact[string](assigned, now), MachineID: UnknownFact[string](selfSrc, now),
			AgeRecipient: UnknownFact[string](selfSrc, now),
		},
		Platform: CapabilityPlatform{
			OS: UnknownFact[string](probeSrc, now), Arch: UnknownFact[string](probeSrc, now), Kernel: UnknownFact[string](probeSrc, now),
			OSVersion: UnknownFact[string](probeSrc, now), Virtualization: UnknownFact[string](probeSrc, now),
		},
		Resources: CapabilityResources{
			CPU: UnknownFact[float64](probeSrc, now), MemMB: UnknownFact[int64](probeSrc, now), DiskFreeMB: UnknownFact[int64](probeSrc, now),
			Accelerators: UnknownFact[[]string](probeSrc, now),
			Reserved:     CapabilityReserved{CPU: UnknownFact[float64](assigned, now), MemMB: UnknownFact[int64](assigned, now)},
		},
		Tools: CapabilityTools{
			Docker: UnknownFact[bool](probeSrc, now), Podman: UnknownFact[bool](probeSrc, now), GVisor: UnknownFact[bool](probeSrc, now),
			Tart: UnknownFact[bool](probeSrc, now), Toolchains: map[string]Fact[string]{}, Browsers: UnknownFact[[]string](probeSrc, now),
			Services: UnknownFact[[]string](probeSrc, now), Custom: map[string]Fact[string]{},
		},
		Availability: CapabilityAvailability{
			State: UnknownFact[string](assigned, now), Slots: UnknownFact[int](selfSrc, now), Running: UnknownFact[int](selfSrc, now),
			BatteryPct: UnknownFact[int](probeSrc, now), PluggedIn: UnknownFact[bool](probeSrc, now), Interactive: UnknownFact[string](probeSrc, now),
			Persistence: UnknownFact[bool](assigned, now), OnBattery: UnknownFact[string](assigned, now), InteractivePolicy: UnknownFact[string](assigned, now),
		},
		Location: CapabilityLocation{Kind: UnknownFact[string](selfSrc, now), Region: UnknownFact[string](selfSrc, now)},
		Economics: CapabilityEconomics{
			Class: UnknownFact[string](selfSrc, now), MarginalCostPerMin: UnknownFact[float64](selfSrc, now),
			Allowance: CapabilityAllowance{
				KnownRemaining: UnknownFact[float64](selfSrc, now), ResetAt: UnknownFact[time.Time](selfSrc, now), Confidence: UnknownFact[string](selfSrc, now),
			},
		},
		Separation: CapabilitySeparation{
			UIDSeparation: UnknownFact[bool](probeSrc, now), UserNS: UnknownFact[bool](probeSrc, now), SigningEligible: UnknownFact[bool](assigned, now),
		},
		Trust: CapabilityTrust{
			Accepts: UnknownFact[[]TrustClass](assigned, now), Isolation: UnknownFact[Isolation](assigned, now),
			Network: UnknownFact[[]NetworkScope](assigned, now), SecretClasses: UnknownFact[[]SecretClass](assigned, now),
			PrivacyZone: UnknownFact[PrivacyZone](assigned, now),
		},
		Verified:  CapabilityVerified{By: selfSrc, At: now},
		Protocol:  CapabilityProtocol{Versions: []int{1}, Min: 1},
		Lifecycle: CapabilityLifecycle{Discovered: true, AuthorizedProjects: []string{}},
	}
}
