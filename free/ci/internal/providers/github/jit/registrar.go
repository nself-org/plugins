package jit

// Purpose: Registrar, the GitHub implementation of providers.RunnerRegistrar (Epic D6).
// Inputs:  Config (the guarded REST client, the three jit settings, an optional fact sink, a clock).
// Outputs: a Registrar. Its REST calls are the generate-jitconfig, runner list and delete, the
//          runner-group read, the branch-protection read and the fork-approval read.
// Constraints: every request goes through rest.Client (SSRF guard, size caps, no retried POST). The
//          encoded JIT config is a credential: it is returned to the caller and nowhere else (no log,
//          no store, no error text). A Registrar has no launch code; HOST-19 owns that.

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	// RunnerPrefix starts the name and the unique label of every runner this package registers.
	RunnerPrefix = "nself-jit-"
	// ConfigLifetime is how long GitHub honours a JIT config that no runner has used.
	ConfigLifetime = 60 * time.Minute
	// OrphanAge is the config life plus ten minutes: a runner older than this is no live job's.
	OrphanAge = ConfigLifetime + 10*time.Minute
	// DefaultGroup is the runner group a repo-level registration lands in.
	DefaultGroup   int64 = 1
	defaultPage          = 100
	maxPages             = 50
	apiVersion           = "2022-11-28"
	factProvider         = "github"
	idCredential         = "github-jit-config"
	workFolderName       = "_work"
)

// FactSink stores provider facts; *store.Store implements it.
type FactSink interface {
	ProviderFactPut(context.Context, store.ProviderFact) (bool, error)
}

// Config builds a Registrar.
type Config struct {
	Client   *rest.Client
	Settings Settings
	Facts    FactSink         // optional: without it FetchApprovalPolicy still returns the policy
	Clock    func() time.Time // default time.Now
	Entropy  io.Reader        // default crypto/rand; tests pin it
	PageSize int              // runners per list page, 1 to 100; default 100
}

// Registrar registers and tears down GitHub JIT runners.
type Registrar struct {
	c        *rest.Client
	set      Settings
	facts    FactSink
	now      func() time.Time
	entropy  io.Reader
	pageSize int
}

var _ providers.RunnerRegistrar = (*Registrar)(nil)

// New checks the settings and returns a Registrar.
func New(cfg Config) (*Registrar, error) {
	if cfg.Client == nil {
		return nil, providers.NewError(providers.Config, "E700", "jit.client", 0, time.Time{})
	}
	set := cfg.Settings
	if set.Label == "" {
		set.Label = DefaultLabel
	}
	if set.Scope == "" {
		set.Scope = ScopeRepo
	}
	if set.Scope != ScopeRepo && set.Scope != ScopeOrg || !labelPattern.MatchString(set.Label) || reservedLabel(set.Label) {
		return nil, providers.NewError(providers.Config, "E700", "jit.settings", 0, time.Time{})
	}
	for _, id := range set.Groups {
		if id <= 0 {
			return nil, providers.NewError(providers.Config, "E700", "jit.settings", 0, time.Time{})
		}
	}
	r := &Registrar{c: cfg.Client, set: set, facts: cfg.Facts, now: cfg.Clock, entropy: cfg.Entropy, pageSize: cfg.PageSize}
	if r.now == nil {
		r.now = time.Now
	}
	if r.pageSize <= 0 || r.pageSize > defaultPage {
		// GitHub caps a page at 100; asking for more would make a full page look short and end the walk.
		r.pageSize = defaultPage
	}
	return r, nil
}

// FromPolicy builds a Registrar from the operator's effective policy: the API URL, token source and
// private-base switch come from the providers.github.* keys, the label, scope and groups from ours.
func FromPolicy(ctx context.Context, eff *policy.Effective, facts FactSink) (*Registrar, error) {
	rs, err := runtime.SettingsFrom(eff)
	if err != nil {
		return nil, err
	}
	js, err := SettingsFrom(eff)
	if err != nil {
		return nil, err
	}
	client := &rest.Client{BaseURL: rs.GitHubAPIURL, Auth: runtime.Credential(ctx, "github"), UserAgent: "jit",
		APIVersion: apiVersion, Provider: "github", AllowPrivateBase: rs.GitHubAllowPrivateBase}
	return New(Config{Client: client, Settings: js, Facts: facts})
}

// ID names the registrar; it is a component of the github provider, not a provider of its own.
func (r *Registrar) ID() string { return "github-jit" }

// Describe reports no capability: runner capacity is described by the github provider (HOST-03).
func (r *Registrar) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	return []model.Capability{}, nil
}

// target is where a request goes: the owner and, at repo scope, the repository.
type target struct{ owner, repo string }

// runnersPath expands the runner collection endpoint of the configured scope.
func (r *Registrar) runnersPath(t target) (string, map[string]string) {
	if r.set.Scope == ScopeOrg {
		return "/orgs/{org}/actions/runners", map[string]string{"org": t.owner}
	}
	return "/repos/{owner}/{repo}/actions/runners", map[string]string{"owner": t.owner, "repo": t.repo}
}

// splitRepo reads "owner/name"; found is false unless both parts are present.
func splitRepo(s string) (owner, repo string, found bool) {
	owner, repo, _ = strings.Cut(s, "/")
	if strings.ContainsAny(s, " ?#") || strings.Contains(repo, "/") || owner == "" || repo == "" {
		return owner, repo, false
	}
	return owner, repo, true
}

// resolve reads "owner/name" from the scope. Org scope needs only the owner part.
func (r *Registrar) resolve(s providers.RunnerScope) (target, error) {
	owner, repo, ok := splitRepo(s.Repo)
	orgOnly := r.set.Scope == ScopeOrg && owner != "" && repo == "" && !strings.ContainsAny(s.Repo, " ?#/")
	if !ok && !orgOnly {
		return target{}, providers.NewError(providers.Config, "E702", "scope.repo", 0, time.Time{})
	}
	return target{owner: owner, repo: repo}, nil
}

// runnerErr names a rejected runner request E707 and leaves the auth, quota and transient classes
// (E701, E703, E704) as the funnel set them, so the caller still sees "missing_scope:admin:org".
func runnerErr(err error, op string) error {
	pe, ok := err.(*providers.Error)
	if !ok || pe.Class != providers.Config || pe.Code != "E702" {
		return err
	}
	return providers.NewError(providers.Config, "E707", op+".rejected", pe.Status, pe.RetryAt)
}

// runnerList is the part of GitHub's runner list this package reads.
type runnerList struct {
	Total   int `json:"total_count"`
	Runners []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Busy bool   `json:"busy"`
	} `json:"runners"`
}

type runnerInfo struct {
	ID   int64
	Name string
	Busy bool
}

// listRunners reads every page of the scope's runners, stopping at a short page or total_count.
func (r *Registrar) listRunners(ctx context.Context, t target) ([]runnerInfo, error) {
	tmpl, params := r.runnersPath(t)
	var all []runnerInfo
	for page := 1; page <= maxPages; page++ {
		var out runnerList
		q := url.Values{"per_page": {strconv.Itoa(r.pageSize)}, "page": {strconv.Itoa(page)}}
		if _, err := r.c.DoQuery(ctx, "GET", tmpl, params, q, nil, &out); err != nil {
			return nil, runnerErr(err, "list")
		}
		for _, x := range out.Runners {
			all = append(all, runnerInfo{ID: x.ID, Name: x.Name, Busy: x.Busy})
		}
		if len(out.Runners) < r.pageSize || out.Total > 0 && len(all) >= out.Total {
			return all, nil
		}
	}
	return nil, providers.NewError(providers.Config, "E707", "list.page_cap", 0, time.Time{})
}
