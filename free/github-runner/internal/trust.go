package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Trusted-only defaults (Epic D23, ADR 0025 section 2). A persistent runner on
// the nSelf host must only run jobs from repositories the operator trusts, so
// before a registration or a start the plugin reads the runner group (or the
// repository) through the GitHub API and refuses one that is open to public
// repositories. Fork pull requests of a public repository would otherwise run
// on this host.

const (
	// CodeRunnerGroupPublic is the stable error code for a public group or repo.
	CodeRunnerGroupPublic = "runner_group_public"
	// CodeRunnerGroupNotFound means the configured group is not in the org.
	CodeRunnerGroupNotFound = "runner_group_not_found"
	// CodeTrustUnverifiable means the check could not be completed. The guard
	// fails closed: a runner that cannot be proven private-only does not run.
	CodeTrustUnverifiable = "runner_trust_unverifiable"
	// CodeRunnerGroupMismatch means the runner is registered in a different
	// group from the configured one, so the group that was checked is not the
	// group the runner takes jobs from. Re-register the runner to move it.
	CodeRunnerGroupMismatch = "runner_group_mismatch"

	groupsPageSize = 100
	maxGroupPages  = 50
)

// githubAPIBase is the GitHub REST root. Tests point it at an httptest server.
var githubAPIBase = "https://api.github.com"

// TrustError is a refusal from the trust guard.
type TrustError struct {
	Code    string
	Status  int
	Message string
}

func (e *TrustError) Error() string { return e.Code + ": " + e.Message }

// TrustConfig is the guard input, read from the environment.
type TrustConfig struct {
	PAT         string
	Org         string
	Scope       string // "org" (default) or "repo"
	Repo        string
	Group       string // empty resolves to the default group
	AllowPublic bool
	RunnerName  string // the runner whose registered group is verified (org scope)
	RunnerDir   string // holds .runner, the persisted registration, which is verified too
	Registering bool   // a registration is about to (re)place the runner: skip the registered-group check
}

// runnerName is the display name the runner is registered under.
func runnerName() string {
	if name := os.Getenv("GITHUB_RUNNER_NAME"); name != "" {
		return name
	}
	hostname, _ := os.Hostname()
	return "nself-" + hostname
}

// TrustConfigFromEnv reads the guard settings from the environment.
func TrustConfigFromEnv() TrustConfig {
	scope := strings.ToLower(strings.TrimSpace(os.Getenv("GITHUB_RUNNER_SCOPE")))
	if scope == "" {
		scope = "org"
	}
	return TrustConfig{
		PAT:         os.Getenv("GITHUB_RUNNER_PAT"),
		Org:         os.Getenv("GITHUB_RUNNER_ORG"),
		Scope:       scope,
		Repo:        os.Getenv("GITHUB_RUNNER_REPO"),
		Group:       strings.TrimSpace(os.Getenv("GITHUB_RUNNER_GROUP")),
		AllowPublic: strings.EqualFold(strings.TrimSpace(os.Getenv("GITHUB_RUNNER_ALLOW_PUBLIC_REPOS")), "true"),
		RunnerName:  runnerName(),
		RunnerDir:   runnerDir(),
	}
}

// TrustResult is the outcome of a passing check.
type TrustResult struct {
	Group    string // resolved group name (org scope)
	Override bool   // public repositories allowed by GITHUB_RUNNER_ALLOW_PUBLIC_REPOS
	Warning  string // set when the override let a public group or repo through
}

type runnerGroup struct {
	ID                       int64  `json:"id"`
	Name                     string `json:"name"`
	Default                  bool   `json:"default"`
	AllowsPublicRepositories *bool  `json:"allows_public_repositories"`
}

// CheckTrust reads the runner group (org scope) or repository (repo scope)
// from GitHub and refuses one open to public repositories unless the override
// is set. It never logs or returns the PAT.
func CheckTrust(ctx context.Context, cfg TrustConfig) (*TrustResult, error) {
	if cfg.PAT == "" || cfg.Org == "" {
		return nil, &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadRequest,
			Message: "GITHUB_RUNNER_PAT and GITHUB_RUNNER_ORG must be set so the runner group can be checked"}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	switch cfg.Scope {
	case "repo":
		return checkRepo(ctx, cfg)
	case "org":
		return checkGroup(ctx, cfg)
	}
	return nil, &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadRequest,
		Message: fmt.Sprintf("GITHUB_RUNNER_SCOPE must be org or repo (got %q)", cfg.Scope)}
}

func checkGroup(ctx context.Context, cfg TrustConfig) (*TrustResult, error) {
	g, err := findGroup(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if g.AllowsPublicRepositories == nil {
		return nil, &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadGateway,
			Message: fmt.Sprintf("GitHub did not report whether runner group %q allows public repositories", g.Name)}
	}
	res := &TrustResult{Group: g.Name, Override: cfg.AllowPublic}
	// The configured group passes (or is overridden): the runner must also be in it.
	registered := func() (*TrustResult, error) {
		if !cfg.Registering {
			if err := verifyRegisteredGroup(ctx, cfg, g); err != nil {
				return nil, err
			}
		}
		return res, nil
	}
	if !*g.AllowsPublicRepositories {
		return registered()
	}
	if cfg.AllowPublic {
		res.Warning = fmt.Sprintf("runner group %q is open to public repositories; GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true lets fork pull requests run on this host", g.Name)
		log.Printf("github-runner: WARNING: %s", res.Warning)
		return registered()
	}
	return nil, &TrustError{Code: CodeRunnerGroupPublic, Status: http.StatusConflict,
		Message: fmt.Sprintf("runner group %q is open to public repositories, so fork pull requests could run on this host. "+
			"Restrict the group to private repositories in the organisation settings, or set GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true to accept the risk", g.Name)}
}

// stopsRunner reports whether a refusal with code must stop a running runner.
func stopsRunner(code string) bool {
	return code == CodeRunnerGroupPublic || code == CodeRunnerGroupMismatch
}

// findGroup pages through the org runner groups and returns the configured
// group, or the group with default=true when none is configured.
func findGroup(ctx context.Context, cfg TrustConfig) (*runnerGroup, error) {
	for page := 1; page <= maxGroupPages; page++ {
		endpoint := fmt.Sprintf("%s/orgs/%s/actions/runner-groups?per_page=%d&page=%d",
			githubAPIBase, url.PathEscape(cfg.Org), groupsPageSize, page)
		var body struct {
			Groups []runnerGroup `json:"runner_groups"`
		}
		if err := githubGet(ctx, cfg.PAT, endpoint, &body); err != nil {
			return nil, err
		}
		for i := range body.Groups {
			g := body.Groups[i]
			if (cfg.Group == "" && g.Default) || (cfg.Group != "" && strings.EqualFold(g.Name, cfg.Group)) {
				return &g, nil
			}
		}
		if len(body.Groups) < groupsPageSize {
			break
		}
	}
	if cfg.Group == "" {
		return nil, &TrustError{Code: CodeRunnerGroupNotFound, Status: http.StatusConflict,
			Message: "no default runner group found in the organisation"}
	}
	return nil, &TrustError{Code: CodeRunnerGroupNotFound, Status: http.StatusConflict,
		Message: fmt.Sprintf("runner group %q not found in the organisation", cfg.Group)}
}

func checkRepo(ctx context.Context, cfg TrustConfig) (*TrustResult, error) {
	if cfg.Repo == "" {
		return nil, &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadRequest,
			Message: "GITHUB_RUNNER_REPO is required for repo scope"}
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s", githubAPIBase, url.PathEscape(cfg.Org), url.PathEscape(cfg.Repo))
	var body struct {
		Private *bool `json:"private"`
	}
	if err := githubGet(ctx, cfg.PAT, endpoint, &body); err != nil {
		return nil, err
	}
	if body.Private == nil {
		return nil, &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadGateway,
			Message: "GitHub did not report whether the repository is private"}
	}
	res := &TrustResult{Override: cfg.AllowPublic}
	if *body.Private {
		return res, nil
	}
	if cfg.AllowPublic {
		res.Warning = fmt.Sprintf("repository %s/%s is public; GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true lets fork pull requests run on this host", cfg.Org, cfg.Repo)
		log.Printf("github-runner: WARNING: %s", res.Warning)
		return res, nil
	}
	return nil, &TrustError{Code: CodeRunnerGroupPublic, Status: http.StatusConflict,
		Message: fmt.Sprintf("repository %s/%s is public, so fork pull requests could run on this host. "+
			"Use a private repository, or set GITHUB_RUNNER_ALLOW_PUBLIC_REPOS=true to accept the risk", cfg.Org, cfg.Repo)}
}

// githubGet performs an authenticated GET and decodes the JSON body into out.
// Any failure becomes a CodeTrustUnverifiable refusal (fail closed). The
// error text never contains the PAT.
func githubGet(ctx context.Context, pat, endpoint string, out interface{}) error {
	unverifiable := func(format string, a ...interface{}) error {
		return &TrustError{Code: CodeTrustUnverifiable, Status: http.StatusBadGateway, Message: fmt.Sprintf(format, a...)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return unverifiable("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// url.Error carries the URL only, never headers.
		var ue *url.Error
		if errors.As(err, &ue) {
			return unverifiable("GitHub API request failed: %v", ue.Err)
		}
		return unverifiable("GitHub API request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return unverifiable("GitHub API returned %d for the runner group check (token needs the organisation \"Self-hosted runners\" permission): %s",
			resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return unverifiable("decode GitHub API response: %v", err)
	}
	return nil
}

// deployHostWarning returns the warning to report in /health and status when
// the stack runs in a deployed environment, else "".
func deployHostWarning() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ENV"))) {
	case "prod", "production", "staging":
		return "this runner shares a host with a deployed nSelf stack: it is a persistent, non-isolated runner. " +
			"Use it for trusted private repositories only; never for fork pull requests"
	}
	return ""
}
