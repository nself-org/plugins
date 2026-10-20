package runnerjob

// Purpose: Minter and RegisterMinter, the seam between the provider-neutral launcher and one forge's
// runner registration (GitHub JIT config today, a GitLab runner token in P7-HOST-07).
// Inputs:  a Minter implementation registered under the lease credential kind it produces.
// Outputs: the process-wide state the preparer, the finalizer and the sweep read; Use binds the ledger.
// Constraints: a Minter returns the credential to its caller and nowhere else (no log, no error text, no
//          store). Deregister must prove the runner absent or return an error. Orphans lists only
//          registrations the minter itself made that no live job can hold. The state is dormant until
//          Use is called (an unbound process runs no runner job and mints nothing).

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/transport"
)

// Request asks a Minter for one single-job registration.
type Request struct {
	Repo      string // owner/name
	Demand    string // the forge's job id
	AttemptID string
	Arch      string // amd64 or arm64
}

// Registration is what a Minter made: the credential for the lease and the handle teardown needs.
type Registration struct {
	Handle     string
	Credential string
	ExpiresAt  time.Time
}

// Minter registers and removes the runners of one forge.
type Minter interface {
	// Provider names the forge, as the pipeline's EnvProvider does.
	Provider() string
	Mint(ctx context.Context, r Request) (Registration, error)
	// Deregister removes the registration and verifies it is gone; absent is success.
	Deregister(ctx context.Context, repo, handle string) error
	// Orphans lists idle registrations of repo born before the cutoff.
	Orphans(ctx context.Context, repo string, before time.Time) ([]string, error)
}

type runtimeState struct {
	mu      sync.RWMutex
	ledger  *Ledger
	minters map[string]Minter // by credential kind
}

var state = &runtimeState{minters: map[string]Minter{}}

// RegisterMinter installs the minter that produces credentials of kind. A repeated kind is refused.
func RegisterMinter(kind string, m Minter) error {
	if !kindPattern.MatchString(kind) || m == nil || m.Provider() == "" {
		return errors.New("runnerjob: a minter needs a plain credential kind and a provider")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if _, dup := state.minters[kind]; dup {
		return errors.New("runnerjob: a minter is already registered for " + kind)
	}
	state.minters[kind] = m
	return nil
}

var installOnce sync.Once
var installErr error

// InstallCoordinator registers the coordinator-side parts once: the transport preparer, the run
// finalizer and the orphan sweep Reconciler. A provider package calls it from its init(); the agent
// binary, which links this package for its hook, never does.
func InstallCoordinator() error {
	installOnce.Do(func() {
		if err := transport.RegisterPreparer(prepare); err != nil {
			installErr = err
			return
		}
		cirun.RegisterFinalizer(finalizer{})
		installErr = coordinator.RegisterReconciler(&Sweeper{})
	})
	return installErr
}

// Use binds the ledger (the coordinator start calls it once, with its store). nil unbinds.
func Use(l *Ledger) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.ledger = l
}

// CurrentLedger returns the bound ledger, or nil.
func CurrentLedger() *Ledger {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.ledger
}

func (s *runtimeState) minterFor(kind string) Minter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.minters[kind]
}

func (s *runtimeState) minterOf(provider string) (kind string, m Minter) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, v := range s.minters {
		if v.Provider() == provider {
			return k, v
		}
	}
	return "", nil
}

// KindOf is the credential kind a provider's minter produces, "" when none is registered. Pass it to
// Restore.
func KindOf(provider string) string {
	k, _ := state.minterOf(provider)
	return k
}

func fail(reason string) error {
	return providers.NewError(providers.Config, "E707", reason, 0, time.Time{})
}
