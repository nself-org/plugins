package hosted

// Purpose: link the provider into the process (Epic D5, Epic D4 wiring): register "github-hosted"
//          with the provider registry, give it the credential rule of the "github" provider, and
//          install the AttemptFacts source on the transport driver of kind "provider".
// Inputs:  init(); UseStore at coordinator start.
// Outputs: providers.Lookup("github-hosted"); runtime credential rule; driver facts.
// Constraints: everything here is dormant unless compat.V15() (Describe, Dispatch and the driver
//          check it); registration failures are kept for RegistrationErrors, never a panic, so a
//          test binary that links two copies of the registry still starts.

import (
	"context"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var defaultProvider = New(Config{})

// Default returns the provider registered with the registry.
func Default() *Provider { return defaultProvider }

func init() {
	if err := providers.Register(defaultProvider); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
	if err := runtime.RegisterCredential(ProviderID, func(ctx context.Context) rest.AuthFunc { return runtime.Credential(ctx, "github") }); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
	runtime.Default().SetFacts(Facts)
}

// UseStore makes the default provider keep its dispatch journal in the coordinator's store (call once
// at coordinator start, next to runtime.UseStore).
func UseStore(st *store.Store) { defaultProvider.UseJournal(NewStoreJournal(st)) }

// ReleaseStore puts the default provider's dispatch journal back in memory (the coordinator's store is closing).
func ReleaseStore() { defaultProvider.UseJournal(NewMemJournal()) }

// UseJournal replaces the dispatch journal.
func (p *Provider) UseJournal(j Journal) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.journal = j
}

func (p *Provider) jrnl() Journal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.journal
}
