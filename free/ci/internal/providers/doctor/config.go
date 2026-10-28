// Package doctor is the `providers` section of `nself ci doctor --providers` (P7-HOST-12, Epic D14,
// X18) and the configuration reader the `nself ci providers` commands share with it.
//
// Purpose: one offline answer to "is hosted capacity set up the way the operator meant": the
// credential sources, the generated workflow on the default and untrusted refs, the per-repo webhook
// secret variables, the stored fork-approval fact, the last health of each provider API and the
// Forgejo URL. Ids CI-DP001 to CI-DP020 belong to this package (CI-50 prefix rule).
// Inputs:  doctor.Env (repository root, the exec funnel, the environment) and the CI store when one
// exists; the operator policy, resolved the way `policy show` resolves it.
// Outputs: doctor.Check values with a remediation each; Config for the commands.
// Constraints: the section sends no request of its own (online checks are `nself ci providers test`); the one
// exception is `gh auth status`, which gh itself answers by asking the API whether its token is valid (CI-DP001
// then warns "not authenticated" when the machine is offline); a credential value is
// read only to learn its kind (the prefix) and is never stored, printed or put in a Detail;
// a variable counts as present when it is non-empty; the section never writes.
package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/forgejo"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/hosted"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	ghwebhook "github.com/nself-org/plugins/free/ci/internal/providers/github/webhook"
	glwebhook "github.com/nself-org/plugins/free/ci/internal/providers/gitlab/webhook"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Config is the provider configuration of one repository under the operator policy. When the policy
// could not be resolved, Err says why and every other field holds its default.
type Config struct {
	Runtime       runtime.Settings
	Hosted        hosted.Settings
	GitHubSecrets map[string]ghwebhook.RepoSecret
	GitLabSecrets glwebhook.Secrets
	JIT           bool     // any providers.github.jit.* key is set
	ForgejoURL    string   // the key's value; forgejo.DefaultURL when unset
	ForgejoSet    bool     // the operator set providers.forgejo.url
	Authorized    []string // trust.providers, effective
	PrivacyZone   string   // trust.privacy_zone, effective ("" when unresolved)
	Set           map[string]bool
	Effective     *policy.Effective // the resolved policy; nil when it could not be resolved
	Err           error
}

// Defaults is the configuration of an operator who set nothing.
func Defaults() Config {
	return Config{Runtime: runtime.DefaultSettings(), Hosted: hosted.DefaultSettings(), ForgejoURL: forgejo.DefaultURL, Set: map[string]bool{}}
}

// Load resolves the operator policy for the repository at root and reads the provider settings from
// it. st may be nil (no operator rows). run is the exec funnel the resolver reads git through.
func Load(ctx context.Context, root string, run policy.Runner, st *store.Store) Config {
	cfg := Defaults()
	run = offline(run)
	res := policy.Resolver{Run: run}
	if st != nil {
		res.Store = st
	}
	got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: policy.ProjectIDGit(ctx, run, root), Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root})
	if err != nil {
		cfg.Err = err
		return cfg
	}
	eff := &got.Effective
	cfg.Effective = eff
	for key := range got.Values {
		cfg.Set[key] = true
	}
	read := func(into func() error) {
		if err := into(); err != nil && cfg.Err == nil {
			cfg.Err = err
		}
	}
	read(func() (e error) { cfg.Runtime, e = runtime.SettingsFrom(eff); return })
	read(func() (e error) { cfg.Hosted, e = hosted.SettingsFrom(eff); return })
	read(func() (e error) { cfg.GitHubSecrets, e = ghwebhook.SecretsFrom(eff); return })
	read(func() (e error) { cfg.GitLabSecrets, e = glwebhook.SecretsFrom(eff); return })
	read(func() (e error) { cfg.ForgejoURL, e = forgejo.URLFrom(eff); return })
	cfg.ForgejoSet = cfg.Set[forgejo.KeyURL]
	cfg.JIT = cfg.Set[jit.KeyLabel] || cfg.Set[jit.KeyScope] || cfg.Set[jit.KeyGroups]
	if v, ok := got.Values["trust.providers"]; ok {
		if list, isList := v.([]any); isList {
			for _, item := range list {
				if s, isStr := item.(string); isStr {
					cfg.Authorized = append(cfg.Authorized, s)
				}
			}
		}
	}
	if v, ok := got.Values["trust.privacy_zone"].(string); ok {
		cfg.PrivacyZone = v
	}
	return cfg
}

// offline refuses every git command that would use the network. The resolver fetches origin's default
// branch to find the protected-branch policy; the providers keys are operator-scope (the branch cannot
// set them), so a stale local tip answers every question this package asks, and no check may send a
// request (the online checks are `nself ci providers test`).
func offline(run policy.Runner) policy.Runner {
	return func(ctx context.Context, argv ...string) (string, int, error) {
		for _, a := range argv {
			if a == "fetch" || a == "pull" || a == "ls-remote" || a == "clone" {
				return "", 1, nil
			}
		}
		return run(report.WithoutLazyFetch(ctx), argv...)
	}
}

// StorePath is the state database: $NSELF_CI_HOME/state.db, else ~/.nself/ci/state.db. "" when the
// home directory is unknown.
func StorePath(getenv func(string) string) string {
	home := getenv("NSELF_CI_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return filepath.Join(home, "state.db")
}

// OpenExisting opens the state database when it exists; (nil, nil) when it does not, so a read
// never creates one.
func OpenExisting(getenv func(string) string) (*store.Store, error) {
	path := StorePath(getenv)
	if path == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return store.Open(path, store.Options{})
}

// GitHubSecretEnv is the variable that holds the webhook secret of repo (owner/name), "" when the
// operator configured none for it.
func (c Config) GitHubSecretEnv(repo string) string {
	return c.GitHubSecrets[strings.ToLower(repo)].Env
}

// GitLabSecretEnvs are the variables of project's signing secret and token, "" for each not configured.
func (c Config) GitLabSecretEnvs(project string) (signing, token string) {
	p := strings.ToLower(project)
	return c.GitLabSecrets.Signing[p].Env, c.GitLabSecrets.Tokens[p].Env
}
