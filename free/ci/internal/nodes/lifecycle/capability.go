package lifecycle

// Purpose: build the capability document of a node `nodes add` is about to register, and apply the
// admin's trust fields to it.
// Inputs:  id, name, destination, platform, pinned fingerprints, separation facts, the admin's flags.
// Outputs: a model.Capability that passes model.ValidateCapability.
// Constraints: facts the CLI did not observe are unknown (null + confidence unknown); trust comes only
//          from the admin's flags (defaults accepts [owner, internal], secret_classes [none], ownership
//          operator, ADR 0025 §2, X5); a node that is not uid-separated accepts [owner] only (D8) and is
//          not signing eligible; nothing a node reports (hello, probe) reaches the trust block.

import (
	"time"

	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func unknown[T any](src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Source: src, ObservedAt: now, Confidence: "unknown"}
}

func known[T any](v T, src string, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: src, ObservedAt: now, Confidence: "known"}
}

// newCapability is the draft the probe fills in.
func newCapability(id, name string, spec remote.HostSpec, p Platform, fps []string, sep SeparationFacts, agentVersion string, now time.Time) model.Capability {
	c := model.NewCapability("probe", "self", now)
	c.Identity = model.CapabilityIdentity{
		ID: id, Name: name, Provider: "ssh", Transport: "ssh", Labels: []string{}, HostKeyFingerprints: fps,
		Ownership: unknown[string]("assigned", now), MachineID: unknown[string]("self", now), AgeRecipient: unknown[string]("self", now),
		SSH: &model.CapabilitySSH{Alias: spec.Host, User: spec.User},
	}
	c.Platform = model.CapabilityPlatform{
		OS: known(p.OS, "probe", now), Arch: known(p.Arch, "probe", now), Kernel: unknown[string]("probe", now),
		OSVersion: unknown[string]("probe", now), Virtualization: unknown[string]("probe", now),
	}
	c.Availability = model.CapabilityAvailability{
		State: known("offline", "assigned", now), Slots: unknown[int]("self", now), Running: unknown[int]("self", now),
		BatteryPct: unknown[int]("probe", now), PluggedIn: unknown[bool]("probe", now), Interactive: unknown[string]("probe", now),
		Persistence: unknown[bool]("assigned", now), OnBattery: unknown[string]("assigned", now), InteractivePolicy: unknown[string]("assigned", now),
	}
	c.Separation = model.CapabilitySeparation{
		UIDSeparation: known(sep.UIDSeparation, "probe", now), UserNS: known(sep.UserNS, "probe", now),
		SigningEligible: known(sep.UIDSeparation, "assigned", now),
	}
	c.SecretsEligible = sep.UIDSeparation
	c.Verified = model.CapabilityVerified{By: "self", At: now} // not "probe": the prober treats a fresh probe as a cache hit
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1, AgentVersion: agentVersion}
	c.Lifecycle = model.CapabilityLifecycle{Discovered: true, AuthorizedProjects: []string{}}
	empty := []model.TrustClass{}
	c.Trust = model.CapabilityTrust{
		Accepts: known(empty, "assigned", now), Isolation: unknown[model.Isolation]("assigned", now),
		Network: unknown[[]model.NetworkScope]("assigned", now), SecretClasses: unknown[[]model.SecretClass]("assigned", now),
		PrivacyZone: unknown[model.PrivacyZone]("assigned", now),
	}
	return c
}

// TrustFlags are the admin's trust choices for a new node.
type TrustFlags struct {
	Accepts       []string
	SecretClasses []string
	PrivacyZone   string
	Ownership     string
	// AcceptsSet is true when the admin passed --accepts (not the default).
	AcceptsSet bool
}

// DefaultTrust is ADR 0025 §2 for personal and LAN nodes.
func DefaultTrust() TrustFlags {
	return TrustFlags{Accepts: []string{"owner", "internal"}, SecretClasses: []string{"none"}, Ownership: "operator"}
}

// applyTrust writes the admin's trust and ownership into c and economics for an operator-owned node.
// A node without uid separation accepts [owner] only; an explicit wider --accepts is refused.
func applyTrust(c *model.Capability, t TrustFlags, now time.Time) error {
	separated := c.Separation.UIDSeparation.Value != nil && *c.Separation.UIDSeparation.Value
	accepts := t.Accepts
	if !separated {
		for _, a := range accepts {
			if a != "owner" {
				if t.AcceptsSet {
					return usage("node %s has no uid separation (D8): it accepts [owner] only, not %q", c.Identity.ID, a)
				}
				accepts = []string{"owner"}
				break
			}
		}
	}
	if !separated {
		for _, sc := range t.SecretClasses {
			if sc != "none" {
				return usage("node %s has no uid separation (D8): it may hold secret class none only, not %q", c.Identity.ID, sc)
			}
		}
	}
	classes := make([]model.TrustClass, 0, len(accepts))
	for _, a := range accepts {
		classes = append(classes, model.TrustClass(a))
	}
	secrets := make([]model.SecretClass, 0, len(t.SecretClasses))
	for _, s := range t.SecretClasses {
		secrets = append(secrets, model.SecretClass(s))
	}
	c.Trust.Accepts = known(classes, "assigned", now)
	c.Trust.SecretClasses = known(secrets, "assigned", now)
	if t.PrivacyZone != "" {
		c.Trust.PrivacyZone = known(model.PrivacyZone(t.PrivacyZone), "assigned", now)
	}
	c.Identity.Ownership = known(t.Ownership, "assigned", now)
	if t.Ownership == "operator" {
		c.Economics.Class = known("owned", "assigned", now)
		c.Economics.MarginalCostPerMin = known(0.0, "assigned", now)
	}
	return nil
}

// validateTrust checks the flag values against the closed v1 lists.
func validateTrust(t TrustFlags) error {
	for _, a := range t.Accepts {
		if !model.TrustClass(a).Valid() {
			return usage("--accepts %q is not owner, collaborator or internal", a)
		}
	}
	for _, s := range t.SecretClasses {
		if !model.SecretClass(s).Valid() {
			return usage("--secret-classes %q is not a secret class", s)
		}
	}
	if t.PrivacyZone != "" && !model.PrivacyZone(t.PrivacyZone).Valid() {
		return usage("--privacy-zone %q is not a privacy zone", t.PrivacyZone)
	}
	switch t.Ownership {
	case "operator", "team", "provider":
	default:
		return usage("--ownership %q is not operator, team or provider", t.Ownership)
	}
	return nil
}
