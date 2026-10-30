package inventory

// Purpose: the registry record of a host the inventory names but the registry does not know (P7-AUTO-04):
//          a discovered candidate, ineligible, with no authorized project and no trust.
// Inputs:  an Entry with an ssh target.
// Outputs: a model.Capability that passes model.ValidateCapability; registry.Register applies the safe
//          trust block and the discovered lifecycle.
// Constraints: no host key is pinned and nothing is enabled (`ci nodes add` owns both, NODE-05); the
//          candidate carries the identity label `inventory` as its source marker (the capability schema
//          has no free-form fact source); the record holds the ssh target and the platform words only.

import (
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// SourceLabel marks a candidate that sync created.
const SourceLabel = "inventory"

func unknown[T any](src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Source: src, ObservedAt: now, Confidence: "unknown"}
}

func known[T any](v T, src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

func optional(v, src string, now time.Time) model.Fact[string] {
	if v == "" {
		return unknown[string](src, now)
	}
	return known(v, src, now)
}

// NewCandidate builds the capability document of a new candidate.
func NewCandidate(e Entry, now time.Time) model.Capability {
	user, host := "", e.SSH
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		user, host = host[:i], host[i+1:]
	}
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity = model.CapabilityIdentity{ID: e.ID, Name: e.ID, Provider: "ssh", Transport: "ssh", Labels: []string{SourceLabel},
		HostKeyFingerprints: []string{}, Ownership: unknown[string]("assigned", now), MachineID: unknown[string]("self", now),
		AgeRecipient: unknown[string]("self", now), SSH: &model.CapabilitySSH{Alias: host, User: user}}
	c.Platform = model.CapabilityPlatform{OS: optional(e.OS, "assigned", now), Arch: optional(e.Arch, "assigned", now),
		Kernel: unknown[string]("probe", now), OSVersion: unknown[string]("probe", now), Virtualization: unknown[string]("probe", now)}
	c.Resources = model.CapabilityResources{CPU: unknown[float64]("probe", now), MemMB: unknown[int64]("probe", now),
		DiskFreeMB: unknown[int64]("probe", now), Accelerators: unknown[[]string]("probe", now),
		Reserved: model.CapabilityReserved{CPU: unknown[float64]("assigned", now), MemMB: unknown[int64]("assigned", now)}}
	c.Tools = model.CapabilityTools{Docker: unknown[bool]("probe", now), Podman: unknown[bool]("probe", now),
		GVisor: unknown[bool]("probe", now), Tart: unknown[bool]("probe", now), Toolchains: map[string]model.Fact[string]{},
		Browsers: unknown[[]string]("probe", now), Services: unknown[[]string]("probe", now), Custom: map[string]model.Fact[string]{}}
	c.Availability = model.CapabilityAvailability{State: known("offline", "assigned", now), Slots: unknown[int]("self", now),
		Running: unknown[int]("self", now), BatteryPct: unknown[int]("probe", now), PluggedIn: unknown[bool]("probe", now),
		Interactive: unknown[string]("probe", now), Persistence: unknown[bool]("assigned", now),
		OnBattery: unknown[string]("assigned", now), InteractivePolicy: unknown[string]("assigned", now)}
	c.Location = model.CapabilityLocation{Kind: unknown[string]("self", now), Region: unknown[string]("self", now)}
	c.Economics = model.CapabilityEconomics{Class: unknown[string]("self", now), MarginalCostPerMin: unknown[float64]("self", now),
		Allowance: model.CapabilityAllowance{KnownRemaining: unknown[float64]("self", now), ResetAt: unknown[time.Time]("self", now),
			Confidence: unknown[string]("self", now)}}
	c.Separation = model.CapabilitySeparation{UIDSeparation: known(false, "assigned", now), UserNS: known(false, "assigned", now),
		SigningEligible: known(false, "assigned", now)}
	c.Verified = model.CapabilityVerified{By: "self", At: now}
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1}
	c.Lifecycle = model.CapabilityLifecycle{Discovered: true, AuthorizedProjects: []string{}}
	return c
}
