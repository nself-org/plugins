package cloudvm

// Purpose: complete the facts of an enrolled burst node (Epic D13, I03): pairing registers a node with
// a generic agent capability, so once the certificate is issued this provider marks the node as what it
// is: a provider-hosted ephemeral VM that belongs to one intent.
// Inputs:  the node id (pairing.Config.AfterIssue) and the registry.
// Outputs: the runner label nself.burst.intent=<token> (the budget step charges the node's job to the
//          intent's reservation), identity provider `ephemeral`, location kind `hosted`, economics
//          class `ephemeral`, and the isolation from burst.trust (ephemeral-vm by default).
// Constraints: a node that is not a burst node of this process is left alone; the trust fields set at
//          approval (accepts, secret classes, privacy zone) are kept; `hosted` is what makes a project in the
//          default privacy zone (private-infrastructure) inadmissible on the node (trust.admitPrivacy), and the
//          provider-allowlist zone admits it only when the operator names provider `ephemeral` in
//          trust.providers; when a step fails, pairing returns the error to the joining node (it never receives
//          its certificate) while the node row and the issued certificate stay: the node has no intent
//          label, so the budget step refuses it any job, and the engine's idle clock ends the intent and
//          revokes both.

import (
	"context"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func fact[T any](v T, p *Provider) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "assigned", ObservedAt: p.cfg.Now().UTC(), Confidence: "known"}
}

// AfterIssue is wired as pairing.Config.AfterIssue (it runs after the node is registered).
func (p *Provider) AfterIssue(ctx context.Context, nodeID string) error {
	token, ok := p.tokenOfNode(nodeID)
	if !ok || p.cfg.Registry == nil {
		return nil
	}
	n, err := p.cfg.Registry.Get(ctx, nodeID)
	if err != nil {
		return fail(ReasonEnroll, err, "node %s cannot be read back", nodeID)
	}
	label := economics.IntentLabelPrefix + token
	c := n.Capability
	if !slices.Contains(c.Identity.Labels, label) {
		c.Identity.Labels = append(slices.Clone(c.Identity.Labels), label)
	}
	c.Identity.Provider = "ephemeral"
	c.Location.Kind = fact("hosted", p)
	c.Economics.Class = fact(economics.ClassEphemeral, p)
	if _, _, err = p.cfg.Registry.UpdateCapability(ctx, nodeID, c, "policy:burst"); err != nil {
		return fail(ReasonEnroll, err, "node %s facts cannot be stored", nodeID)
	}
	pol, err := p.cfg.Spend(ctx)
	if err != nil {
		return fail(ReasonEnroll, err, "the spend policy could not be read")
	}
	t := c.Trust
	t.Isolation = fact(model.Isolation(pol.Burst.Trust.Isolation), p)
	admin, err := store.NewAdminAuthority("policy:burst")
	if err != nil {
		return fail(ReasonEnroll, err, "no authority for the isolation fact")
	}
	if _, err = p.cfg.Registry.SetTrust(ctx, nodeID, t, admin); err != nil {
		return fail(ReasonEnroll, err, "node %s isolation cannot be stored", nodeID)
	}
	return nil
}
