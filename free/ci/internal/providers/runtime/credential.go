package runtime

// Purpose: Credential(ctx, provider) resolves a provider's token source from the operator policy
// (Epic D2): GitHub reads the variable named by providers.github.token_env, else gh's own login for
// the API host; GitLab reads providers.gitlab.token_env. A provider with no built-in rule (a fake
// or a later provider) registers its own with RegisterCredential.
// Inputs:  the provider id and the operator settings.
// Outputs: a rest.AuthFunc that resolves the token at call time and caches it for five minutes.
// Constraints: the token is returned to the REST client only; this package never logs, stores or
//          returns it in an error. A missing credential is E700 (providers.Error), which the world
//          contributor reads as "contributes no runner".

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

var credentials = struct {
	sync.Mutex
	custom map[string]func(context.Context) rest.AuthFunc
	memo   map[string]rest.AuthFunc
}{custom: map[string]func(context.Context) rest.AuthFunc{}, memo: map[string]rest.AuthFunc{}}

// RegisterCredential installs the credential rule of a provider that has no built-in one. A repeated
// provider id is refused.
func RegisterCredential(provider string, fn func(context.Context) rest.AuthFunc) error {
	credentials.Lock()
	defer credentials.Unlock()
	if provider == "github" || provider == "gitlab" {
		return providers.NewError(providers.Config, "E700", "the credential rule of "+provider+" comes from the operator policy and cannot be replaced", 0, time.Time{})
	}
	if provider == "" || fn == nil {
		return providers.NewError(providers.Config, "E700", "credential rule needs a provider and a function", 0, time.Time{})
	}
	if _, ok := credentials.custom[provider]; ok {
		return providers.NewError(providers.Config, "E700", "credential rule already registered: "+provider, 0, time.Time{})
	}
	credentials.custom[provider] = fn
	return nil
}

func missing(reason string) rest.AuthFunc {
	return func(context.Context) (string, error) {
		return "", providers.NewError(providers.Config, "E700", reason, 0, time.Time{})
	}
}

// Credential returns the token source of provider under the current operator policy. It never fails:
// an unresolvable source is an AuthFunc that returns E700 when asked.
func Credential(ctx context.Context, provider string) rest.AuthFunc {
	credentials.Lock()
	custom := credentials.custom[provider]
	credentials.Unlock()
	if custom != nil {
		return custom(ctx)
	}
	s, err := CurrentSettings(ctx)
	if err != nil {
		return missing("provider policy unreadable")
	}
	var env, host string
	switch provider {
	case "github":
		env, host = s.GitHubTokenEnv, ghHost(s.GitHubAPIURL)
		if env == "" {
			return memoized("github|gh|"+host, func() rest.AuthFunc { return rest.GHToken(host) })
		}
	case "gitlab":
		env = s.GitLabTokenEnv
	default:
		return missing("no credential rule for provider " + provider)
	}
	if env == "" {
		return missing("token_env")
	}
	return memoized(provider+"|env|"+env, func() rest.AuthFunc { return rest.EnvToken(env) })
}

// memoized keeps one AuthFunc per source so the five minute token cache survives between calls
// (a fresh GHToken per call would run gh every time).
func memoized(key string, build func() rest.AuthFunc) rest.AuthFunc {
	credentials.Lock()
	defer credentials.Unlock()
	if fn, ok := credentials.memo[key]; ok {
		return fn
	}
	fn := build()
	credentials.memo[key] = fn
	return fn
}

// ghHost is the host gh knows the login by: api.github.com is github.com, a GHES API URL
// (https://ghe.example/api/v3) is its own host.
func ghHost(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	if h := strings.ToLower(u.Hostname()); h != "api.github.com" {
		return h
	}
	return "github.com"
}
