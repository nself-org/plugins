package lifecycle

// Purpose: the machine and human view of a node for `nodes list|show|add|...`: discovered, authorized,
// eligible and revoked stay separate fields (ADR 0025 §6, §10).
// Inputs:  a registry.Node.
// Outputs: NodeView, the data of the v1 envelope.
// Constraints: no key material and no token: only fingerprints (public), ids, states and trust fields.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
)

// TrustView is the admin-assigned trust of a node.
type TrustView struct {
	Accepts       []string `json:"accepts"`
	SecretClasses []string `json:"secret_classes"`
	PrivacyZone   *string  `json:"privacy_zone"`
	Isolation     *string  `json:"isolation"`
}

// SeparationView is the node's separation block (Epic D8).
type SeparationView struct {
	UIDSeparation   *bool `json:"uid_separation"`
	UserNS          *bool `json:"userns"`
	SigningEligible *bool `json:"signing_eligible"`
}

// LabelView is one operator-assigned label (P7-NODE-30); labels are never self-declared.
type LabelView struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// IdentityView is the assigned part of a node's identity.
type IdentityView struct {
	Labels []LabelView `json:"labels"`
}

// NodeView is one node.
type NodeView struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	State               string         `json:"state"`
	Reason              string         `json:"reason"`
	Transport           string         `json:"transport"`
	Platform            string         `json:"platform"`
	Ownership           *string        `json:"ownership"`
	Discovered          bool           `json:"discovered"`
	AuthorizedProjects  []string       `json:"authorized_projects"`
	Eligible            bool           `json:"eligible"`
	RevokedAt           *time.Time     `json:"revoked_at"`
	Identity            IdentityView   `json:"identity"`
	Trust               TrustView      `json:"trust"`
	Separation          SeparationView `json:"separation"`
	AgentVersion        string         `json:"agent_version"`
	HostKeyFingerprints []string       `json:"host_key_fingerprints"`
	VerifiedBy          string         `json:"verified_by"`
	VerifiedAt          time.Time      `json:"verified_at"`
}

func strs[T ~string](v *[]T) []string {
	out := []string{}
	if v == nil {
		return out
	}
	for _, x := range *v {
		out = append(out, string(x))
	}
	return out
}

func ptrStr[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

// acceptsView renders the accepts set; null (a provisioned GitHub runner, source provider, P7-NODE-06)
// stays JSON null instead of looking like an empty admin choice.
func acceptsView(v *[]model.TrustClass) []string {
	if v == nil {
		return nil
	}
	return strs(v)
}

// ViewOf renders a node.
func ViewOf(n registry.Node) NodeView {
	c := n.Capability
	plat := ""
	if c.Platform.OS.Value != nil && c.Platform.Arch.Value != nil {
		plat = *c.Platform.OS.Value + "/" + *c.Platform.Arch.Value
	}
	projects := c.Lifecycle.AuthorizedProjects
	if projects == nil {
		projects = []string{}
	}
	fps := c.Identity.HostKeyFingerprints
	if fps == nil {
		fps = []string{}
	}
	return NodeView{
		ID: n.Record.ID, Name: n.Record.Name, State: n.Record.State, Reason: n.Record.Reason, Transport: c.Identity.Transport,
		Platform: plat, Ownership: c.Identity.Ownership.Value, Discovered: c.Lifecycle.Discovered, AuthorizedProjects: projects,
		Eligible: c.Lifecycle.Eligible, RevokedAt: c.Lifecycle.RevokedAt, Identity: IdentityView{Labels: []LabelView{}},
		Trust: TrustView{
			Accepts: acceptsView(c.Trust.Accepts.Value), SecretClasses: strs(c.Trust.SecretClasses.Value),
			PrivacyZone: ptrStr[model.PrivacyZone](c.Trust.PrivacyZone.Value), Isolation: ptrStr[model.Isolation](c.Trust.Isolation.Value),
		},
		Separation:   SeparationView{UIDSeparation: c.Separation.UIDSeparation.Value, UserNS: c.Separation.UserNS.Value, SigningEligible: c.Separation.SigningEligible.Value},
		AgentVersion: c.Protocol.AgentVersion, HostKeyFingerprints: fps, VerifiedBy: c.Verified.By, VerifiedAt: c.Verified.At,
	}
}

// withLabels fills identity.labels from the assigned labels in the store (source assigned).
func (d *Deps) withLabels(ctx context.Context, v NodeView) (NodeView, error) {
	set, err := d.Store.NodeLabels(ctx, v.ID)
	if err != nil {
		return v, err
	}
	for _, l := range set {
		v.Identity.Labels = append(v.Identity.Labels, LabelView{Key: l.Key, Value: l.Value, Source: "assigned"})
	}
	return v, nil
}
