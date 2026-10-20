package runners

// Purpose: Registrar, the GitLab implementation of providers.RunnerRegistrar (Epic D11).
// Inputs:  Config (the guarded REST client, the runner settings, a clock, an entropy source).
// Outputs: a Registrar. Its REST calls are the project read, POST /user/runners, the runner read, delete
//          and pause, the project runner list, a runner's running jobs, and the job and protection reads
//          behind Restriction.
// Constraints: every request goes through rest.Client (SSRF guard, size caps, no retried POST). The client
//          carries the runner-management token only (providers.gitlab.runner_token_env, a token with the
//          create_runner and manage_runner scopes whose user is Maintainer of the project): FromPolicy
//          never falls back to the Developer token of the dispatch provider, so a missing runner token is
//          E700 and nothing else is tried. The glrt- authentication token is a credential: it is returned
//          to the caller and nowhere else (no log, no store, no error text). A Registrar has no launch
//          code; HOST-19's runnerjob owns that.

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/gitlab/api"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

const (
	// RunnerPrefix starts the description and the unique tag of every runner this package creates.
	RunnerPrefix = "nself-glr-"
	// CredentialKind is the lease credential kind of a GitLab runner authentication token.
	CredentialKind = "gitlab-runner-token"
	// TokenPrefix is what every runner authentication token starts with (the legacy registration
	// tokens, removed in GitLab 18.0, do not).
	TokenPrefix = "glrt-"
	// CredentialLifetime bounds the lease credential: the runner job limit (six hours) plus ten minutes.
	CredentialLifetime = 6*time.Hour + 10*time.Minute
	// OrphanAge is how old an idle runner must be before the sweep removes it: the runner waits at most
	// 120 s for its job, so anything idle after this is nobody's.
	OrphanAge = 70 * time.Minute
)

// The GitLab routes this package calls ({id} is the URL-encoded project path or number).
const (
	userRunnersPath     = "/user/runners"
	runnerPath          = "/runners/{runner}"
	runnerJobsPath      = "/runners/{runner}/jobs"
	projectRunnersPath  = "/projects/{id}/runners"
	jobPath             = "/projects/{id}/jobs/{job}"
	protectedBranchPath = "/projects/{id}/protected_branches/{name}"
	protectedTagPath    = "/projects/{id}/protected_tags/{name}"
)

// Config builds a Registrar.
type Config struct {
	Client   *rest.Client
	Settings Settings         // zero value: DefaultSettings
	Clock    func() time.Time // default time.Now
	Entropy  io.Reader        // default crypto/rand; tests pin it
}

// Registrar creates and tears down GitLab project runners.
type Registrar struct {
	c       *rest.Client
	set     Settings
	now     func() time.Time
	entropy io.Reader
}

var _ providers.RunnerRegistrar = (*Registrar)(nil)
var _ providers.Provider = (*Registrar)(nil)

// New checks the settings and returns a Registrar.
func New(cfg Config) (*Registrar, error) {
	if cfg.Client == nil {
		return nil, providers.NewError(providers.Config, "E700", "runner.client", 0, time.Time{})
	}
	set := cfg.Settings
	if len(set.Tags) == 0 {
		set.Tags = DefaultSettings().Tags
	}
	if err := checkTags(set.Tags); err != nil {
		return nil, providers.NewError(providers.Config, "E700", "runner.tags", 0, time.Time{})
	}
	r := &Registrar{c: cfg.Client, set: set, now: cfg.Clock, entropy: cfg.Entropy}
	r.set.Tags = append([]string(nil), set.Tags...)
	if r.now == nil {
		r.now = time.Now
	}
	return r, nil
}

// NewClient builds the REST client of the runner-management token from the operator's GitLab settings.
func NewClient(rs runtime.Settings) *rest.Client {
	return &rest.Client{BaseURL: rs.GitLabAPIURL, Auth: rest.EnvToken(rs.GitLabRunnerTokenEnv), UserAgent: "runners",
		Provider: "gitlab", AllowPrivateBase: rs.GitLabAllowPrivateBase}
}

// FromPolicy builds a Registrar from the operator's effective policy.
func FromPolicy(eff *policy.Effective) (*Registrar, error) {
	rs, err := runtime.SettingsFrom(eff)
	if err != nil {
		return nil, err
	}
	set, err := SettingsFrom(eff)
	if err != nil {
		return nil, err
	}
	return New(Config{Client: NewClient(rs), Settings: set})
}

// ID names the registrar; it is a component of the gitlab provider, not a provider of its own.
func (r *Registrar) ID() string { return "gitlab-runners" }

// Describe reports no capability: GitLab capacity is described by the gitlab provider (HOST-06).
func (r *Registrar) Describe(context.Context, providers.Query) ([]model.Capability, error) {
	return []model.Capability{}, nil
}

// project reads the path of a scope.
func project(s providers.RunnerScope) (api.Project, error) {
	p, ok := api.ParseProject(s.Repo)
	if !ok {
		return api.Project{}, e702("scope.repo")
	}
	return p, nil
}

// projectID resolves the numeric id POST /user/runners wants.
func (r *Registrar) projectID(ctx context.Context, p api.Project) (int64, error) {
	var info api.ProjectInfo
	if _, err := r.c.Do(ctx, "GET", api.ProjectPath, p.Params(), nil, &info); err != nil {
		return 0, runnerErr(err, "project")
	}
	if info.ID <= 0 {
		return 0, providers.NewError(providers.Config, "E707", "project.malformed", 0, time.Time{})
	}
	return info.ID, nil
}

// runnerErr names a rejected runner request E707 and leaves the auth, quota and transient classes
// (E700, E701, E703, E704) as the funnel set them, so the caller still sees the permission it lacks.
func runnerErr(err error, op string) error {
	pe, ok := err.(*providers.Error)
	if !ok || pe.Class != providers.Config || pe.Code != "E702" {
		return err
	}
	return providers.NewError(providers.Config, "E707", op+".rejected", pe.Status, pe.RetryAt)
}

func e707(reason string) error {
	return providers.NewError(providers.Config, "E707", reason, 0, time.Time{})
}

func e702(reason string) error {
	return providers.NewError(providers.Config, "E702", reason, 0, time.Time{})
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
