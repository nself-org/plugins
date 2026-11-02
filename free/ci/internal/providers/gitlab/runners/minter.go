package runners

// Purpose: Minter, the runnerjob.Minter for GitLab: it adapts the Registrar so the launcher mints, removes
// and lists runners without knowing GitLab.
// Inputs:  a Registrar source (default: built from the operator policy on each call).
// Outputs: Registration{Handle: the runner id, Credential: the glrt- token, ExpiresAt}; Deregister proves
//          the runner absent; Orphans lists idle nself-glr-<ulid> runners.
// Constraints: every registration is for an untrusted attempt (a project runner is never governed by a
//          group, so nothing proves a job safe: review F6); the demand's own tags are not copied: the
//          runner carries the operator's tags that the job asks for (read from the API by Assess), plus its unique
//          one, never an architecture tag that contradicts the runner's. Only a demand whose ref is
//          proved protected may launch a runner, and every runner is ref_protected. A failed proof fails
//          closed: an unrestricted token stolen by an unprotected shell job could request protected work.
//          The credential goes back to the caller and nowhere else.

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/runnerjob"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

// Minter is the GitLab runnerjob.Minter.
type Minter struct {
	// Registrar returns the registrar; nil builds one from the operator policy for each call.
	Registrar func(context.Context) (*Registrar, error)
}

var _ runnerjob.Minter = (*Minter)(nil)

// Provider is "gitlab".
func (*Minter) Provider() string { return Source }

var override struct {
	sync.RWMutex
	fn func(context.Context) (*Registrar, error)
}

// UseRegistrar replaces where the default Minter gets its registrar (a coordinator that builds one itself,
// or a test with a fake forge). nil restores the policy-built one.
func UseRegistrar(fn func(context.Context) (*Registrar, error)) {
	override.Lock()
	defer override.Unlock()
	override.fn = fn
}

func (m *Minter) registrar(ctx context.Context) (*Registrar, error) {
	return m.registrarWithOptionalConfig(ctx, false)
}

// registrarIfConfigured declines running hooks when the operator configured no runner token.
func (m *Minter) registrarIfConfigured(ctx context.Context) (*Registrar, error) {
	return m.registrarWithOptionalConfig(ctx, true)
}

func (m *Minter) registrarWithOptionalConfig(ctx context.Context, optional bool) (*Registrar, error) {
	if m.Registrar != nil {
		return m.Registrar(ctx)
	}
	override.RLock()
	fn := override.fn
	override.RUnlock()
	if fn != nil {
		return fn(ctx)
	}
	rs, err := runtime.CurrentSettings(ctx)
	if err != nil {
		return nil, providers.NewError(providers.Config, "E700", "policy.unreadable", 0, time.Time{})
	}
	set, err := CurrentSettings(ctx) // read per call: the handler reads the tags per demand, so a changed list applies to the next runner
	if err != nil {
		return nil, providers.NewError(providers.Config, "E700", "policy.unreadable", 0, time.Time{})
	}
	if optional && rs.GitLabRunnerTokenEnv == "" {
		return nil, nil
	}
	c, err := NewClient(rs)
	if err != nil {
		return nil, err
	}
	return New(Config{Client: c, Settings: set})
}

// Mint creates one single-job runner for the project.
func (m *Minter) Mint(ctx context.Context, r runnerjob.Request) (runnerjob.Registration, error) {
	reg, err := m.registrar(ctx)
	if err != nil {
		return runnerjob.Registration{}, err
	}
	if reg == nil || !reg.set.ProtectedRefsOnly {
		return runnerjob.Registration{}, refuse("E708", "runner.protected_refs_only_opt_in_required")
	}
	scope := providers.RunnerScope{Repo: r.Repo}
	job, err := strconv.ParseInt(r.Demand, 10, 64)
	if err != nil || job <= 0 {
		return runnerjob.Registration{}, e702("demand.job")
	}
	a, err := reg.Assess(ctx, scope, job)
	if err != nil {
		return runnerjob.Registration{}, err
	}
	if !a.ProtectedOnly {
		return runnerjob.Registration{}, refuse("E708", "runner.protected_ref_required")
	}
	opt := MintOptions{Tags: a.Tags, Arch: r.Arch}
	minted, err := reg.MintRunner(ctx, providers.RunnerRequest{
		Attempt: providers.Attempt{ID: r.AttemptID, Repo: r.Repo, Trust: "untrusted"}, Scope: scope,
	}, opt)
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

// Orphans lists the idle runners of repo born before the cutoff.
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
