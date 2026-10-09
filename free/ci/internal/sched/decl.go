package sched

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

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
