package doctor

// Purpose: where each provider credential comes from and what kind it is, without ever holding the
// credential beyond the prefix test (Epic D14, review F18).
// Inputs:  Config (the token_env keys), the environment reader, the exec funnel for `gh auth status`.
// Outputs: Credential values: source, presence and kind. TokenKind maps a token prefix to a kind.
// Constraints: the value read from the environment is used only to take its kind and is not kept;
// `gh auth status` prints the token masked after its prefix, so the gh kind comes from that prefix
// and `gh auth token` is never run; a missing gh is "not present", not an error.

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

// The credential kinds.
const (
	KindFineGrained = "fine-grained" // github_pat_
	KindClassic     = "classic"      // ghp_
	KindOAuth       = "oauth"        // gho_, the token `gh auth login` stores
	KindApp         = "app"          // ghs_, ghu_, ghr_
	KindGitLabPAT   = "gitlab-pat"   // glpat-
	KindRunner      = "runner"       // glrt-
	KindOther       = "other"
)

// Credential says where a token comes from and what it is. Env is the variable name for an env
// source, "" for gh.
type Credential struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Source   string `json:"source"`
	Env      string `json:"env,omitempty"`
	Present  bool   `json:"present"`
	Kind     string `json:"kind,omitempty"`
}

// TokenKind classifies a token by its prefix. It returns KindOther for an unknown prefix.
func TokenKind(token string) string {
	switch t := strings.TrimSpace(token); {
	case strings.HasPrefix(t, "github_pat_"):
		return KindFineGrained
	case strings.HasPrefix(t, "ghp_"):
		return KindClassic
	case strings.HasPrefix(t, "gho_"):
		return KindOAuth
	case strings.HasPrefix(t, "ghs_"), strings.HasPrefix(t, "ghu_"), strings.HasPrefix(t, "ghr_"):
		return KindApp
	case strings.HasPrefix(t, "glpat-"):
		return KindGitLabPAT
	case strings.HasPrefix(t, "glrt-"):
		return KindRunner
	}
	return KindOther
}

// WeakGitHubKind reports a kind the operator should replace with a fine-grained token.
func WeakGitHubKind(kind string) bool { return kind == KindClassic || kind == KindOAuth }

var maskedToken = regexp.MustCompile(`Token: (github_pat_|ghp_|gho_|ghs_|ghu_|ghr_)`)

// GitHubHost is the host gh knows the login by (api.github.com is github.com).
func GitHubHost(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil || u.Hostname() == "" {
		return "github.com"
	}
	if h := strings.ToLower(u.Hostname()); h != "api.github.com" {
		return h
	}
	return "github.com"
}

// GitHubCredential resolves the GitHub source: the variable named by providers.github.token_env, else
// gh's own login for the API host.
func GitHubCredential(ctx context.Context, cfg Config, getenv func(string) string, capture doctor.CaptureFunc) Credential {
	c := Credential{Provider: "github", Key: "providers.github.token_env"}
	if env := cfg.Runtime.GitHubTokenEnv; env != "" {
		c.Source, c.Env = "env:"+env, env
		if v := strings.TrimSpace(getenv(env)); v != "" {
			c.Present, c.Kind = true, TokenKind(v)
		}
		return c
	}
	c.Source = "gh"
	if capture == nil {
		return c
	}
	out, exit, err := capture(ctx, "gh", "auth", "status", "--hostname", GitHubHost(cfg.Runtime.GitHubAPIURL))
	if err != nil || exit != 0 {
		return c
	}
	c.Present, c.Kind = true, KindOAuth // gh stores the token of its own login flow
	if m := maskedToken.FindStringSubmatch(out); m != nil {
		c.Kind = TokenKind(m[1])
	}
	return c
}

// GitLabCredential resolves one GitLab token variable. runner selects providers.gitlab.runner_token_env
// (no default) over providers.gitlab.token_env.
func GitLabCredential(cfg Config, getenv func(string) string, runner bool) Credential {
	c := Credential{Provider: "gitlab", Key: "providers.gitlab.token_env"}
	env := cfg.Runtime.GitLabTokenEnv
	if runner {
		c.Key, env = "providers.gitlab.runner_token_env", cfg.Runtime.GitLabRunnerTokenEnv
	}
	if env == "" {
		c.Source = "none"
		return c
	}
	c.Source, c.Env = "env:"+env, env
	if v := strings.TrimSpace(getenv(env)); v != "" {
		c.Present, c.Kind = true, TokenKind(v)
	}
	return c
}

// GitLabConfigured reports whether the operator set any providers.gitlab.* key.
func GitLabConfigured(cfg Config) bool {
	for key := range cfg.Set {
		if strings.HasPrefix(key, "providers.gitlab.") {
			return true
		}
	}
	return false
}

// Auth returns the token source for family ("github" or "gitlab") under cfg: the variable named by the
// operator's key, else (GitHub only) gh's own login. runner selects the GitLab runner token variable.
// It exists because the runtime package reads the policy from a source the coordinator installs, which
// a one-shot CLI process does not have; the rules are the same ones (runtime.Credential).
func Auth(cfg Config, family string, runner bool) rest.AuthFunc {
	switch family {
	case "github":
		if env := cfg.Runtime.GitHubTokenEnv; env != "" {
			return rest.EnvToken(env)
		}
		return rest.GHToken(GitHubHost(cfg.Runtime.GitHubAPIURL))
	case "gitlab":
		env := cfg.Runtime.GitLabTokenEnv
		if runner {
			env = cfg.Runtime.GitLabRunnerTokenEnv
		}
		return rest.EnvToken(env)
	}
	return func(context.Context) (string, error) { return "", nil }
}
