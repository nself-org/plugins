package pipeline

// Purpose: link the provider into the process (Epic D11, Epic D4 wiring): register "gitlab" with the
//          provider registry and offer the seam the GitLab trigger source plugs its TrustFacts into.
// Inputs:  init(); UseStore and UseTrustFacts at coordinator start.
// Outputs: providers.Lookup("gitlab"); Refetch, which providers.RefetchFacts finds through the
//          registry under the trigger facts' source name.
// Constraints: everything here is dormant unless compat.V15() (Describe and Dispatch check it);
//          registration failures are kept for RegistrationErrors, never a panic, so a test binary that
//          links two copies of the registry still starts; the credential rule of "gitlab" is built
//          into runtime.Credential, so none is registered here; the AttemptFacts source of the transport
//          driver is installed by the GitHub provider (it reads repo, project and runner label from
//          the node's labels, which this provider sets the same way) and this package does not install
//          a second one; with no TrustFacts installed Refetch returns the facts unchanged, which is
//          what the registry does for a source with no provider at all.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var (
	defaultProvider    = New(Config{})
	registrationErrors []error
)

// Default returns the provider registered with the registry.
func Default() *Provider { return defaultProvider }

// RegistrationErrors returns what init() could not register; empty when everything is registered.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	if err := providers.Register(defaultProvider); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
}

// UseStore makes the default provider keep its dispatch journal in the coordinator's store (call once
// at coordinator start, next to runtime.UseStore).
func UseStore(st *store.Store) { defaultProvider.UseJournal(NewStoreJournal(st)) }

// ReleaseStore puts the default provider's dispatch journal back in memory (the coordinator's store is closing).
func ReleaseStore() { defaultProvider.UseJournal(NewMemJournal()) }

var _ providers.TrustFacts = (*Provider)(nil)

// UseTrustFacts installs the refetcher of trigger facts from the GitLab API (P7-HOST-21 calls this from
// the webhook package). Passing nil removes it.
func (p *Provider) UseTrustFacts(t providers.TrustFacts) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trust = t
}

// Refetch implements providers.TrustFacts by delegating to the installed refetcher.
func (p *Provider) Refetch(ctx context.Context, f model.TriggerFacts) (model.TriggerFacts, error) {
	p.mu.Lock()
	t := p.trust
	p.mu.Unlock()
	if t == nil {
		return f, nil
	}
	return t.Refetch(ctx, f)
}
