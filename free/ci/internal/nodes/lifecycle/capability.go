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

// newCapability is the draft the probe fills in: the shared all-unknown scaffold (model.NewCapability)
// plus what `nodes add` already knows.
func newCapability(id, name string, spec remote.HostSpec, p Platform, fps []string, sep SeparationFacts, agentVersion string, now time.Time) model.Capability {
	c := model.NewCapability("probe", "self", now)
	c.Identity.ID, c.Identity.Name, c.Identity.Transport = id, name, "ssh"
	c.Identity.HostKeyFingerprints = fps
	c.Identity.SSH = &model.CapabilitySSH{Alias: spec.Host, User: spec.User}
	c.Platform.OS, c.Platform.Arch = model.KnownFact(p.OS, "probe", now), model.KnownFact(p.Arch, "probe", now)
	c.Availability.State = model.KnownFact("offline", "assigned", now)
	c.Separation = model.CapabilitySeparation{
		UIDSeparation: model.KnownFact(sep.UIDSeparation, "probe", now), UserNS: model.KnownFact(sep.UserNS, "probe", now),
		SigningEligible: model.KnownFact(sep.UIDSeparation, "assigned", now),
	}
	c.SecretsEligible = sep.UIDSeparation
	c.Protocol.AgentVersion = agentVersion
	c.Trust.Accepts = model.KnownFact([]model.TrustClass{}, "assigned", now)
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
	c.Trust.Accepts = model.KnownFact(classes, "assigned", now)
	c.Trust.SecretClasses = model.KnownFact(secrets, "assigned", now)
	if t.PrivacyZone != "" {
		c.Trust.PrivacyZone = model.KnownFact(model.PrivacyZone(t.PrivacyZone), "assigned", now)
	}
	c.Identity.Ownership = model.KnownFact(t.Ownership, "assigned", now)
	if t.Ownership == "operator" {
		c.Economics.Class = model.KnownFact("owned", "assigned", now)
		c.Economics.MarginalCostPerMin = model.KnownFact(0.0, "assigned", now)
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
