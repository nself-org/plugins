package demand

// Purpose: Minter, the runnerjob.Minter for GitHub: it adapts the JIT registrar (P7-HOST-04) so the
// launcher mints, removes and lists runners without knowing GitHub.
// Inputs:  a Registrar source (default: built from the operator policy on first use).
// Outputs: Registration{Handle: the registrar's runner ref id, Credential: the encoded JIT config,
//          ExpiresAt}; Deregister proves the runner absent; Orphans lists idle nself-jit-<ulid> runners.
// Constraints: every registration is for an untrusted attempt (a repo-scope runner is never governed by a
//          group, so nothing proves a job safe: review F6); the demand's labels are not passed on, only the
//          architecture, so the registrar's fixed labels are the whole label set. The credential goes
//          back to the caller and nowhere else.

import (
	"context"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// Minter is the GitHub runnerjob.Minter.
type Minter struct {
	// Registrar returns the JIT registrar; nil builds one from the operator policy for each call.
	Registrar func(context.Context) (*jit.Registrar, error)
}

var _ runnerjob.Minter = (*Minter)(nil)

// Provider is "github".
func (*Minter) Provider() string { return Source }

var override struct {
	sync.RWMutex
	fn func(context.Context) (*jit.Registrar, error)
}

// UseRegistrar replaces where the default Minter gets its JIT registrar (a coordinator that builds
// one itself, or a test with a fake forge). nil restores the policy-built one.
func UseRegistrar(fn func(context.Context) (*jit.Registrar, error)) {
	override.Lock()
	defer override.Unlock()
	override.fn = fn
}

func (m *Minter) registrar(ctx context.Context) (*jit.Registrar, error) {
	if m.Registrar != nil {
		return m.Registrar(ctx)
	}
	override.RLock()
	fn := override.fn
	override.RUnlock()
	if fn != nil {
		return fn(ctx)
	}
	eff, err := currentPolicy(ctx)
	if err != nil {
		return nil, providers.NewError(providers.Config, "E700", "policy.unreadable", 0, time.Time{})
	}
	if eff != nil { // built per call: the handler reads the jit label per demand, so a changed label applies to the next runner
		return jit.FromPolicy(ctx, eff, nil)
	}
	rs := runtime.DefaultSettings()
	client := &rest.Client{BaseURL: rs.GitHubAPIURL, Auth: runtime.Credential(ctx, Source), UserAgent: "jit", APIVersion: "2022-11-28", Provider: Source}
	return jit.New(jit.Config{Client: client, Settings: jit.DefaultSettings()})
}

// Mint registers one single-job runner for the repository.
func (m *Minter) Mint(ctx context.Context, r runnerjob.Request) (runnerjob.Registration, error) {
	reg, err := m.registrar(ctx)
	if err != nil {
		return runnerjob.Registration{}, err
	}
	arch := "x64"
	if r.Arch == "arm64" {
		arch = "arm64"
	}
	minted, err := reg.MintRunner(ctx, providers.RunnerRequest{
		Attempt: providers.Attempt{ID: r.AttemptID, Repo: r.Repo, Trust: "untrusted"},
		Scope:   providers.RunnerScope{Repo: r.Repo},
		Labels:  []string{arch},
	})
	if err != nil {
		return runnerjob.Registration{}, err
	}
	return runnerjob.Registration{Handle: minted.Ref().ID, Credential: minted.Credential.Value, ExpiresAt: minted.Credential.ExpiresAt}, nil
}

// Deregister removes the runner and proves it gone.
func (m *Minter) Deregister(ctx context.Context, repo, handle string) error {
	reg, err := m.registrar(ctx)
	if err != nil {
		return err
	}
	return reg.Deregister(ctx, providers.RunnerRef{ID: handle, Scope: providers.RunnerScope{Repo: repo}})
}

// Orphans lists the idle registrations of repo born before the cutoff.
func (m *Minter) Orphans(ctx context.Context, repo string, before time.Time) ([]string, error) {
	reg, err := m.registrar(ctx)
	if err != nil {
		return nil, err
	}
	refs, err := reg.Orphans(ctx, providers.RunnerScope{Repo: repo}, before)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out, nil
}
