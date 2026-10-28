package model

// Purpose: the one constructor of a capability document draft, so the three producers (nodes add,
// runner provision, agent pairing) share their Fact scaffolding.
// Inputs:  the source labels for node-reported facts and for the machine identity, and the clock.
// Outputs: a Capability in which every fact is unknown (null + confidence unknown) and every
//          list or map is non-nil, ready for the caller to set what it observed or was told.
// Constraints: the draft carries no identity, trust, verification, protocol or lifecycle values; a
//          caller must set those. It is not valid until the caller does, so ValidateCapability is
//          the caller's gate.

import "time"

// UnknownFact is a fact nobody has observed: no value, confidence unknown.
func UnknownFact[T any](src string, now time.Time) Fact[T] {
	return Fact[T]{Source: src, ObservedAt: now, Confidence: "unknown"}
}

// KnownFact is a fact with a value and confidence known.
func KnownFact[T any](v T, src string, now time.Time) Fact[T] {
	return Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

// NewCapability returns the all-unknown draft. reported is the source of facts the node itself reports
// (probe, self or provider); selfSrc is the source of the machine id and age recipient. Facts the
// operator assigns carry source "assigned".
func NewCapability(reported, selfSrc string, now time.Time) Capability {
	c := Capability{Schema: "ci.runner-capability/v1"}
	c.Identity = CapabilityIdentity{
		Labels: []string{}, HostKeyFingerprints: []string{},
		Ownership: UnknownFact[string]("assigned", now), MachineID: UnknownFact[string](selfSrc, now),
		AgeRecipient: UnknownFact[string](selfSrc, now),
	}
	c.Platform = CapabilityPlatform{
		OS: UnknownFact[string](reported, now), Arch: UnknownFact[string](reported, now), Kernel: UnknownFact[string](reported, now),
		OSVersion: UnknownFact[string](reported, now), Virtualization: UnknownFact[string](reported, now),
	}
	c.Resources = CapabilityResources{
		CPU: UnknownFact[float64](reported, now), MemMB: UnknownFact[int64](reported, now), DiskFreeMB: UnknownFact[int64](reported, now),
		Accelerators: UnknownFact[[]string](reported, now),
		Reserved:     CapabilityReserved{CPU: UnknownFact[float64]("assigned", now), MemMB: UnknownFact[int64]("assigned", now)},
	}
	c.Tools = CapabilityTools{
		Docker: UnknownFact[bool](reported, now), Podman: UnknownFact[bool](reported, now), GVisor: UnknownFact[bool](reported, now), Tart: UnknownFact[bool](reported, now),
		Toolchains: map[string]Fact[string]{}, Browsers: UnknownFact[[]string](reported, now), Services: UnknownFact[[]string](reported, now),
		Custom: map[string]Fact[string]{},
	}
	c.Availability = CapabilityAvailability{
		State: UnknownFact[string]("assigned", now), Slots: UnknownFact[int](selfSrc, now), Running: UnknownFact[int](selfSrc, now),
		BatteryPct: UnknownFact[int](reported, now), PluggedIn: UnknownFact[bool](reported, now), Interactive: UnknownFact[string](reported, now),
		Persistence: UnknownFact[bool]("assigned", now), OnBattery: UnknownFact[string]("assigned", now), InteractivePolicy: UnknownFact[string]("assigned", now),
	}
	c.Location = CapabilityLocation{Kind: UnknownFact[string](selfSrc, now), Region: UnknownFact[string](selfSrc, now)}
	c.Economics = CapabilityEconomics{
		Class: UnknownFact[string](selfSrc, now), MarginalCostPerMin: UnknownFact[float64](selfSrc, now),
		Allowance: CapabilityAllowance{KnownRemaining: UnknownFact[float64](selfSrc, now), ResetAt: UnknownFact[time.Time](selfSrc, now), Confidence: UnknownFact[string](selfSrc, now)},
	}
	c.Separation = CapabilitySeparation{
		UIDSeparation: UnknownFact[bool](reported, now), UserNS: UnknownFact[bool](reported, now), SigningEligible: UnknownFact[bool]("assigned", now),
	}
	c.Lifecycle = CapabilityLifecycle{AuthorizedProjects: []string{}}
	c.Trust = CapabilityTrust{
		Accepts: UnknownFact[[]TrustClass]("assigned", now), Isolation: UnknownFact[Isolation]("assigned", now),
		Network: UnknownFact[[]NetworkScope]("assigned", now), SecretClasses: UnknownFact[[]SecretClass]("assigned", now),
		PrivacyZone: UnknownFact[PrivacyZone]("assigned", now),
	}
	return c
}
