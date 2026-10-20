package webhook

// Purpose: the provider "github" that providers.RefetchFacts routes to. RefetchFacts finds a
// TrustFacts implementation by looking up a Provider under the facts' Source, so the source needs a
// provider of its own to be reachable. This one contributes no capacity (Describe returns nothing, it
// is not a Dispatcher): hosted dispatch is the separate provider "github-hosted" (HOST-03).
// Inputs:  facts to refetch.
// Outputs: Refetch delegates to the Source.
// Constraints: registered once from init() in source.go.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Provider is the registered face of the source for trust-fact refetch.
type Provider struct{ src *Source }

// ID is "github".
func (Provider) ID() string { return Name }

// Describe contributes no capability.
func (Provider) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	return nil, nil
}

// Refetch delegates to the source.
func (p Provider) Refetch(ctx context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
	return p.src.Refetch(ctx, f)
}

var (
	_ providers.Provider   = Provider{}
	_ providers.TrustFacts = Provider{}
)
