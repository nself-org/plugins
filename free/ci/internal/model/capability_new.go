package model

import "time"

// NewCapability initializes a capability with unknown resource, tool, location and
// economics facts. Callers supply identity, platform, availability and trust
// according to their enrollment path before validating the document.
func NewCapability(source, reportedSource string, now time.Time) Capability {
	c := Capability{Schema: "ci.runner-capability/v1"}
	c.Resources = CapabilityResources{
		CPU: unknownCapabilityFact[float64](source, now), MemMB: unknownCapabilityFact[int64](source, now), DiskFreeMB: unknownCapabilityFact[int64](source, now),
		Accelerators: unknownCapabilityFact[[]string](source, now),
		Reserved:     CapabilityReserved{CPU: unknownCapabilityFact[float64]("assigned", now), MemMB: unknownCapabilityFact[int64]("assigned", now)},
	}
	c.Tools = CapabilityTools{
		Docker: unknownCapabilityFact[bool](source, now), Podman: unknownCapabilityFact[bool](source, now), GVisor: unknownCapabilityFact[bool](source, now), Tart: unknownCapabilityFact[bool](source, now),
		Toolchains: map[string]Fact[string]{}, Browsers: unknownCapabilityFact[[]string](source, now), Services: unknownCapabilityFact[[]string](source, now),
		Custom: map[string]Fact[string]{},
	}
	c.Location = CapabilityLocation{Kind: unknownCapabilityFact[string](reportedSource, now), Region: unknownCapabilityFact[string](reportedSource, now)}
	c.Economics = CapabilityEconomics{
		Class: unknownCapabilityFact[string](reportedSource, now), MarginalCostPerMin: unknownCapabilityFact[float64](reportedSource, now),
		Allowance: CapabilityAllowance{KnownRemaining: unknownCapabilityFact[float64](reportedSource, now), ResetAt: unknownCapabilityFact[time.Time](reportedSource, now), Confidence: unknownCapabilityFact[string](reportedSource, now)},
	}
	return c
}

func unknownCapabilityFact[T any](source string, now time.Time) Fact[T] {
	return Fact[T]{Source: source, ObservedAt: now, Confidence: "unknown"}
}
