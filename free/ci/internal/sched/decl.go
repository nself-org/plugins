package sched

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// RunnerHosted reports whether r is a provider-hosted runner: the one predicate placement
// (eligibility), settlement and delivery share, so a runner is hosted for all of them or none.
// Ephemeral alone does not make a runner hosted: a burst or ephemeral node still runs the agent.
func RunnerHosted(r Runner) bool {
	k := r.Capability.Location.Kind.Value
	return r.Location == "hosted" || k != nil && *k == "hosted" || r.Decl.Hosted
}

// DeclarationFromCapability maps coordinator-assigned capability facts to admission facts.
func DeclarationFromCapability(c model.Capability, host bool, mode trust.CoordinatorMode) trust.Declaration {
	d := trust.Declaration{CoordinatorHost: host, CoordinatorMode: mode, Provider: c.Identity.Provider,
		Hosted:     c.Location.Kind.Value != nil && *c.Location.Kind.Value == "hosted",
		DeployHost: trust.DeployHost{Matched: c.DeployHost.Matched, Override: c.DeployHost.Override}}
	if v := c.Trust.Accepts.Value; v != nil {
		d.Accepts = append([]model.TrustClass(nil), (*v)...)
	}
	if v := c.Trust.Isolation.Value; v != nil {
		d.Isolation = *v
	}
	if v := c.Trust.Network.Value; v != nil && len(*v) > 0 {
		d.Network = (*v)[len(*v)-1]
	}
	if v := c.Trust.SecretClasses.Value; v != nil {
		d.SecretClasses = append([]model.SecretClass(nil), (*v)...)
	}
	if v := c.Trust.PrivacyZone.Value; v != nil {
		d.PrivacyZone = *v
	}
	if v := c.Identity.Ownership.Value; v != nil {
		d.Ownership = *v
	}
	if v := c.Separation.UIDSeparation.Value; v != nil {
		d.UIDSeparation = *v
	}
	return d
}
