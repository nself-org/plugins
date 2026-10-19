package provision

// Purpose: the registry record of a provisioned GitHub runner (P7-NODE-06, review F14). A runner takes
// its jobs from GitHub, not from an nSelf lease, so the node is recorded with trust.accepts null from
// source "provider": no trust class is ever admitted and the scheduler never offers it a lease.
// Inputs:  the host label of the provisioned machine and the runner labels.
// Outputs: a validated model.Capability document and the node id derived from the host.
// Constraints: the record holds the host name and the labels only, never the registration token, the
// GitHub URL or any host output; the id is lower case, `gh-runner-` plus the host with unsafe bytes
// replaced; the document passes model.ValidateCapability.

import (
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// RecordRequest is what the composition root records after a provision succeeded.
type RecordRequest struct {
	// Host is the ssh target as given ("user@host").
	Host   string
	Labels []string
}

// RunnerNodeID is the registry id of the runner host.
func RunnerNodeID(host string) string {
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		host = host[i+1:]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(host) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return "gh-runner-" + strings.Trim(b.String(), "-.")
}

func unknownFact[T any](src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Source: src, ObservedAt: now, Confidence: "unknown"}
}

func knownFact[T any](v T, src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

// RunnerCapability builds the capability document of a provisioned runner host.
func RunnerCapability(req RecordRequest, now time.Time) model.Capability {
	id := RunnerNodeID(req.Host)
	user, host := "", req.Host
	if i := strings.LastIndexByte(host, '@'); i >= 0 {
		user, host = host[:i], host[i+1:]
	}
	labels := append([]string{"github-runner"}, req.Labels...)
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity = model.CapabilityIdentity{
		ID: id, Name: id, Provider: "ssh", Transport: "provider", Labels: labels, HostKeyFingerprints: []string{},
		Ownership: knownFact("operator", "assigned", now), MachineID: unknownFact[string]("provider", now),
		AgeRecipient: unknownFact[string]("provider", now), SSH: &model.CapabilitySSH{Alias: host, User: user},
	}
	c.Platform = model.CapabilityPlatform{OS: knownFact("linux", "assigned", now), Arch: unknownFact[string]("provider", now),
		Kernel: unknownFact[string]("provider", now), OSVersion: unknownFact[string]("provider", now), Virtualization: unknownFact[string]("provider", now)}
	c.Resources = model.CapabilityResources{CPU: unknownFact[float64]("provider", now), MemMB: unknownFact[int64]("provider", now),
		DiskFreeMB: unknownFact[int64]("provider", now), Accelerators: unknownFact[[]string]("provider", now),
		Reserved: model.CapabilityReserved{CPU: unknownFact[float64]("assigned", now), MemMB: unknownFact[int64]("assigned", now)}}
	c.Tools = model.CapabilityTools{Docker: unknownFact[bool]("provider", now), Podman: unknownFact[bool]("provider", now),
		GVisor: unknownFact[bool]("provider", now), Tart: unknownFact[bool]("provider", now), Toolchains: map[string]model.Fact[string]{},
		Browsers: unknownFact[[]string]("provider", now), Services: unknownFact[[]string]("provider", now), Custom: map[string]model.Fact[string]{}}
	c.Availability = model.CapabilityAvailability{State: knownFact("online", "assigned", now), Slots: unknownFact[int]("provider", now),
		Running: unknownFact[int]("provider", now), BatteryPct: unknownFact[int]("provider", now), PluggedIn: unknownFact[bool]("provider", now),
		Interactive: unknownFact[string]("provider", now), Persistence: knownFact(true, "assigned", now),
		OnBattery: unknownFact[string]("assigned", now), InteractivePolicy: unknownFact[string]("assigned", now)}
	c.Location = model.CapabilityLocation{Kind: unknownFact[string]("provider", now), Region: unknownFact[string]("provider", now)}
	c.Economics = model.CapabilityEconomics{Class: unknownFact[string]("provider", now), MarginalCostPerMin: unknownFact[float64]("provider", now),
		Allowance: model.CapabilityAllowance{KnownRemaining: unknownFact[float64]("provider", now), ResetAt: unknownFact[time.Time]("provider", now),
			Confidence: unknownFact[string]("provider", now)}}
	c.Separation = model.CapabilitySeparation{UIDSeparation: knownFact(false, "assigned", now), UserNS: knownFact(false, "assigned", now),
		SigningEligible: knownFact(false, "assigned", now)}
	c.Verified = model.CapabilityVerified{By: "provider", At: now}
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1}
	c.Lifecycle = model.CapabilityLifecycle{Discovered: true, AuthorizedProjects: []string{}}
	c.Trust = model.CapabilityTrust{
		// null from source provider: the one shape ValidateCapability allows for an unknown accepts set.
		Accepts:   model.Fact[[]model.TrustClass]{Source: "provider", ObservedAt: now, Confidence: "unknown"},
		Isolation: unknownFact[model.Isolation]("assigned", now), Network: unknownFact[[]model.NetworkScope]("assigned", now),
		SecretClasses: unknownFact[[]model.SecretClass]("assigned", now), PrivacyZone: unknownFact[model.PrivacyZone]("assigned", now)}
	return c
}
