package main

// Purpose: `nself ci providers test <id> [--repo R] [--json]` (P7-HOST-12, Epic D14): the online check
// of one provider's setup. It sends read requests only and reports, per check, ok, warn, fail,
// unknown or skip: the credential resolves (source, presence, kind), the scopes the API reports
// (classic tokens send them in a header; fine-grained and App tokens send none, which reads as
// unknown), access to the repository and whether the token has admin on it, the dispatch workflow on
// the default and untrusted refs (GitHub), the per-repo webhook secret variable, and for GitLab the
// token's own record with the role it holds and the scopes of the runner token (providers_probe_gitlab.go).
// Inputs:  the provider id, the repository (flag, else the origin remote), the operator policy.
// Outputs: one row per check, a text table or one v1 JSON envelope {provider, repo, checks, ok}; exit 0
//          when no check failed, 1 when one did.
// Constraints: read only, every request through the REST funnel (SSRF guard, redirect allowlist,
//          size caps); a credential value is never printed or put in a row, only its source, presence
//          and kind; errors are shown by their provider code and reason, which hold no token.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	cidoc "github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/store"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
	gitlabtmpl "github.com/nself-org/plugins/free/ci/templates/gitlab"
)

// The row statuses of a test.
const (
	probeOK      = "ok"
	probeWarn    = "warn"
	probeFail    = "fail"
	probeUnknown = "unknown"
	probeSkip    = "skip"
)

type probeRow struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

var githubRepoName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*/[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func productionProviderClient(cfg pdoctor.Config, family string, runner bool) (*rest.Client, error) {
	switch family {
	case "github":
		hosts, err := githubtmpl.RedirectHosts()
		if err != nil {
			return nil, err
		}
		return &rest.Client{BaseURL: cfg.Runtime.GitHubAPIURL, Auth: pdoctor.Auth(cfg, family, false), RedirectHosts: hosts,
			UserAgent: "providers-test", APIVersion: "2022-11-28", Provider: "github", AllowPrivateBase: cfg.Runtime.GitHubAllowPrivateBase}, nil
	case "gitlab":
		hosts, err := gitlabtmpl.RedirectHosts()
		if err != nil {
			return nil, err
		}
		return &rest.Client{BaseURL: cfg.Runtime.GitLabAPIURL, Auth: pdoctor.Auth(cfg, family, runner), RedirectHosts: hosts,
			UserAgent: "providers-test", Provider: "gitlab", AllowPrivateBase: cfg.Runtime.GitLabAllowPrivateBase}, nil
	}
	return nil, fmt.Errorf("no REST client for provider family %q", family)
}

// providerFailure names a provider error by its code and reason; any other error by its text.
func providerFailure(err error) string {
	var pe *providers.Error
	if errors.As(err, &pe) {
		return strings.TrimSpace(fmt.Sprintf("%s %s (status %d)", pe.Code, pe.Reason, pe.Status))
	}
	return oneLineMsg(err)
}

func testProvider(ctx context.Context, env providersEnv, cfg pdoctor.Config, st *store.Store, root, id string, f providersFlags) int {
	const command = "ci providers test"
	if _, ok := providers.Lookup(id); !ok {
		return adminFail(env.adminEnv, command, f.json, unknownProvider(id))
	}
	repo := f.repo
	var rows []probeRow
	switch health.Family(id) {
	case "github":
		if repo == "" {
			repo, _ = pdoctor.GitHubRepo(ctx, root, cidoc.CaptureFunc(env.Run))
		}
		rows = probeGitHub(ctx, env, cfg, st, repo)
	case "gitlab":
		rows = probeGitLab(ctx, env, cfg, repo)
	default:
		rows = []probeRow{{"online test", probeSkip, "no online test exists for provider family " + health.Family(id)}}
	}
	failed := false
	for _, r := range rows {
		failed = failed || r.Status == probeFail
	}
	if f.json {
		if code := adminWriteData(env.adminEnv, command, map[string]any{"provider": id, "repo": repo, "checks": rows, "ok": !failed}); code != 0 {
			return code
		}
	} else {
		for _, r := range rows {
			_, _ = fmt.Fprintf(env.Out, "%-8s %-28s %s\n", r.Status, r.Name, plainText(r.Detail))
		}
	}
	if failed {
		return 1
	}
	return 0
}

type ghRepoInfo struct {
	DefaultBranch string `json:"default_branch"`
	Permissions   struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

func probeGitHub(ctx context.Context, env providersEnv, cfg pdoctor.Config, st *store.Store, repo string) []probeRow {
	cred := pdoctor.GitHubCredential(ctx, cfg, env.Getenv, cidoc.CaptureFunc(env.Run))
	if !cred.Present {
		return []probeRow{{"credential", probeFail, "no GitHub credential resolves (" + cred.Source + "); online checks skipped"}}
	}
	rows := []probeRow{{"credential", probeOK, cred.Source + ", kind " + cred.Kind}}
	if pdoctor.WeakGitHubKind(cred.Kind) {
		rows = append(rows, probeRow{"credential kind", probeWarn, "a " + cred.Kind + " token has broader access than a fine-grained github_pat_ token"})
	}
	c, err := env.NewClient(cfg, "github", false)
	if err != nil {
		return append(rows, probeRow{"api client", probeFail, oneLineMsg(err)})
	}
	var user struct {
		Login string `json:"login"`
	}
	resp, err := c.Do(ctx, "GET", "/user", nil, nil, &user)
	if err != nil {
		return append(rows, probeRow{"token", probeFail, providerFailure(err)})
	}
	rows = append(rows, probeRow{"token", probeOK, "authenticated as " + user.Login})
	if resp.ScopesSet {
		rows = append(rows, probeRow{"scopes", probeOK, "[" + strings.TrimSpace(resp.Scopes) + "]"})
	} else {
		rows = append(rows, probeRow{"scopes", probeUnknown, "the API reported none (fine-grained and App tokens send no scopes header)"})
	}
	if !githubRepoName.MatchString(repo) {
		return append(rows, probeRow{"repository", probeSkip, "pass --repo owner/name (no GitHub origin remote found)"})
	}
	return append(rows, probeGitHubRepo(ctx, c, cfg, st, repo, env)...)
}

func probeGitHubRepo(ctx context.Context, c *rest.Client, cfg pdoctor.Config, st *store.Store, repo string, env providersEnv) []probeRow {
	owner, name, _ := strings.Cut(repo, "/")
	params := map[string]string{"owner": owner, "repo": name}
	var info ghRepoInfo
	if _, err := c.Do(ctx, "GET", "/repos/{owner}/{repo}", params, nil, &info); err != nil {
		return []probeRow{{"repository access", probeFail, repo + ": " + providerFailure(err)}}
	}
	rows := []probeRow{{"repository access", probeOK, fmt.Sprintf("%s: pull %v, push %v", repo, info.Permissions.Pull, info.Permissions.Push)}}
	if info.Permissions.Admin {
		rows = append(rows, probeRow{"repository admin", probeOK, "the token has admin on " + repo})
	} else {
		rows = append(rows, probeRow{"repository admin", probeWarn, "the token has no admin on " + repo + " (only just-in-time runner registration needs it)"})
	}
	for _, ref := range []struct{ label, branch string }{{"workflow, default branch", info.DefaultBranch}, {"workflow, untrusted ref", cfg.Hosted.UntrustedRef}} {
		rows = append(rows, probeWorkflow(ctx, c, params, ref.label, ref.branch))
	}
	if cfg.JIT {
		rows = append(rows, probeForkApproval(ctx, c, cfg, st, repo))
	}
	return append(rows, probeSecretEnv("webhook secret", cfg.GitHubSecretEnv(repo), env))
}

// probeForkApproval reads the repository's fork pull request approval policy and records it as the
// host-provider fact github/<repo>/fork_pr_approval_policy, which CI-DP010 of the doctor reads (nothing
// else writes it). It runs only when just-in-time runners are enabled; the one write is that fact.
func probeForkApproval(ctx context.Context, c *rest.Client, cfg pdoctor.Config, st *store.Store, repo string) probeRow {
	const name = "fork pr approval"
	settings := jit.DefaultSettings()
	if cfg.Effective != nil {
		if s, err := jit.SettingsFrom(cfg.Effective); err == nil {
			settings = s
		}
	}
	jc := jit.Config{Client: c, Settings: settings}
	if st != nil {
		jc.Facts = st
	}
	reg, err := jit.New(jc)
	if err != nil {
		return probeRow{name, probeFail, providerFailure(err)}
	}
	policyName, err := reg.FetchApprovalPolicy(ctx, providers.RunnerScope{Repo: repo})
	switch {
	case err != nil:
		return probeRow{name, probeFail, providerFailure(err)}
	case policyName != jit.StrictApproval:
		return probeRow{name, probeWarn, repo + " approves fork pull requests under " + policyName + ", weaker than " + jit.StrictApproval + " (just-in-time runners are enabled)"}
	}
	return probeRow{name, probeOK, repo + " requires approval for all external contributors"}
}

func probeWorkflow(ctx context.Context, c *rest.Client, params map[string]string, label, branch string) probeRow {
	var doc struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	path := "/repos/{owner}/{repo}/contents/" + githubtmpl.Path
	resp, err := c.DoQuery(ctx, "GET", path, params, url.Values{"ref": {branch}}, nil, &doc)
	switch {
	case err != nil && resp.Status == 404:
		return probeRow{label, probeFail, githubtmpl.Path + " is missing on " + branch + " (run `nself ci providers github init --check`)"}
	case err != nil:
		return probeRow{label, probeFail, providerFailure(err)}
	}
	raw, derr := base64.StdEncoding.DecodeString(strings.ReplaceAll(doc.Content, "\n", ""))
	switch {
	case derr != nil || doc.Encoding != "base64":
		return probeRow{label, probeUnknown, "the file on " + branch + " could not be read"}
	case !strings.HasPrefix(string(raw), githubtmpl.HeaderMarker):
		return probeRow{label, probeWarn, "the file on " + branch + " is not generated by `nself ci providers github init`"}
	case githubtmpl.VersionOf(raw) != githubtmpl.Current:
		return probeRow{label, probeWarn, fmt.Sprintf("the file on %s is template v%d, this build renders v%d", branch, githubtmpl.VersionOf(raw), githubtmpl.Current)}
	}
	return probeRow{label, probeOK, "present on " + branch + ", template v" + fmt.Sprint(githubtmpl.Current)}
}

// probeSecretEnv reports whether the variable that holds a webhook secret is set ("" means the
// operator configured none for this repository).
func probeSecretEnv(name, envVar string, env providersEnv) probeRow {
	switch {
	case envVar == "":
		return probeRow{name, probeUnknown, "no per-repository secret is configured (the shared secret applies)"}
	case strings.TrimSpace(env.Getenv(envVar)) == "":
		return probeRow{name, probeFail, envVar + " is empty or unset"}
	}
	return probeRow{name, probeOK, envVar + " is set"}
}
