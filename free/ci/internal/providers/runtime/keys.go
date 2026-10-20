// Package runtime wires hosted providers into the fabric's chokepoints (P7-HOST-16, Epic D4):
// the operator-scope policy keys, the transport Driver of kind "provider", the world contributor
// "providers", the hosted lease profiles and the ingress adapter for trigger sources. A provider
// package only implements internal/providers; this package is the one place that connects it.
package runtime

// Purpose: the providers.* policy keys the runtime owns and their typed view (Epic X1).
// Inputs:  *policy.Effective from the P7-TRUST-27 resolver (UsePolicy installs the source).
// Outputs: Settings; the keys register from init().
// Constraints: operator scope only. Every key carries Spend: true, which is how the policy engine
//          keeps a key out of the protected branch, the revision, the pipeline and a run: a
//          repository must never choose the API host a token is sent to or the variable a token is
//          read from. Decoders are strict (a JSON string or boolean, never a coerced value); a bad
//          value is E672. Values hold names and URLs, never a token.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyGitHubAPIURL           = "providers.github.api_url"
	KeyGitHubTokenEnv         = "providers.github.token_env"
	KeyGitHubAllowPrivateBase = "providers.github.allow_private_base"
	KeyGitLabAPIURL           = "providers.gitlab.api_url"
	KeyGitLabTokenEnv         = "providers.gitlab.token_env"
	KeyGitLabRunnerTokenEnv   = "providers.gitlab.runner_token_env"
	KeyGitLabAllowPrivateBase = "providers.gitlab.allow_private_base"
)

// Defaults (Epic §Contracts).
const (
	DefaultGitHubAPIURL   = "https://api.github.com"
	DefaultGitLabAPIURL   = "https://gitlab.com/api/v4"
	DefaultGitLabTokenEnv = "NSELF_CI_GITLAB_TOKEN"
	keyOwner              = "P7-HOST-16"
)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every key is in the
// process-wide key set.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

// RegisterKeys adds the seven keys to the process-wide set and returns what failed.
func RegisterKeys() []error {
	var errs []error
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Keys returns the spec of every runtime key. Kind is TightenFixed: once the operator sets a key, no
// later scope may change it. The registered Default is nil on purpose: the trust registry seeds a
// non-nil default as an existing value, and a Fixed key could then never be set by anyone (E670).
// The defaults live in DefaultSettings, which SettingsFrom applies to every key the operator left
// unset.
func Keys() []policy.KeySpec {
	k := func(key string, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Decode: dec}
	}
	return []policy.KeySpec{
		k(KeyGitHubAPIURL, decodeURL(KeyGitHubAPIURL)),
		k(KeyGitHubTokenEnv, decodeEnvName(KeyGitHubTokenEnv)),
		k(KeyGitHubAllowPrivateBase, decodeBool(KeyGitHubAllowPrivateBase)),
		k(KeyGitLabAPIURL, decodeURL(KeyGitLabAPIURL)),
		k(KeyGitLabTokenEnv, decodeEnvName(KeyGitLabTokenEnv)),
		k(KeyGitLabRunnerTokenEnv, decodeEnvName(KeyGitLabRunnerTokenEnv)),
		k(KeyGitLabAllowPrivateBase, decodeBool(KeyGitLabAllowPrivateBase)),
	}
}

func init() { registrationErrors = RegisterKeys() }

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeOne(key string, raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, fail(key, "malformed JSON")
	}
	if d.More() || v == nil {
		return nil, fail(key, "expected exactly one value")
	}
	return v, nil
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// decodeEnvName accepts a variable name or "" (no variable: the provider's own default applies).
func decodeEnvName(key string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeOne(key, raw)
		if err != nil {
			return nil, err
		}
		s, ok := v.(string)
		if !ok || s != "" && !envName.MatchString(s) {
			return nil, fail(key, "expected an environment variable name such as MY_TOKEN, or an empty string")
		}
		return s, nil
	}
}

// decodeURL accepts an https base URL with a host, no user info, query or fragment (the REST funnel
// refuses any other scheme, so a value it would refuse is refused here, at policy load).
func decodeURL(key string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeOne(key, raw)
		if err != nil {
			return nil, err
		}
		s, ok := v.(string)
		if !ok {
			return nil, fail(key, "expected a URL string")
		}
		u, err := url.Parse(s)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" {
			return nil, fail(key, "expected an https base URL with a host and no user info, query or fragment")
		}
		return strings.TrimRight(s, "/"), nil
	}
}

func decodeBool(key string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeOne(key, raw)
		if err != nil {
			return nil, err
		}
		b, ok := v.(bool)
		if !ok {
			return nil, fail(key, "expected true or false")
		}
		return b, nil
	}
}

// Settings is the typed view of the seven keys.
type Settings struct {
	GitHubAPIURL, GitHubTokenEnv                       string
	GitLabAPIURL, GitLabTokenEnv, GitLabRunnerTokenEnv string
	GitHubAllowPrivateBase, GitLabAllowPrivateBase     bool
}

// DefaultSettings is what an operator who set nothing gets.
func DefaultSettings() Settings {
	return Settings{GitHubAPIURL: DefaultGitHubAPIURL, GitLabAPIURL: DefaultGitLabAPIURL, GitLabTokenEnv: DefaultGitLabTokenEnv}
}

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("providers: no effective policy")
	}
	s := DefaultSettings()
	var err error
	str := func(key string, into *string) {
		if v, _, ok := eff.Value(key); ok && err == nil {
			got, isStr := v.(string)
			if !isStr {
				err = fail(key, "effective value %v is not a string", v)
				return
			}
			*into = got
		}
	}
	flag := func(key string, into *bool) {
		if v, _, ok := eff.Value(key); ok && err == nil {
			got, isBool := v.(bool)
			if !isBool {
				err = fail(key, "effective value %v is not a boolean", v)
				return
			}
			*into = got
		}
	}
	str(KeyGitHubAPIURL, &s.GitHubAPIURL)
	str(KeyGitHubTokenEnv, &s.GitHubTokenEnv)
	flag(KeyGitHubAllowPrivateBase, &s.GitHubAllowPrivateBase)
	str(KeyGitLabAPIURL, &s.GitLabAPIURL)
	str(KeyGitLabTokenEnv, &s.GitLabTokenEnv)
	str(KeyGitLabRunnerTokenEnv, &s.GitLabRunnerTokenEnv)
	flag(KeyGitLabAllowPrivateBase, &s.GitLabAllowPrivateBase)
	return s, err
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this
// once). With none installed the defaults apply. Passing nil removes it.
func UsePolicy(fn func(context.Context) (*policy.Effective, error)) {
	policySource.Lock()
	defer policySource.Unlock()
	policySource.fn = fn
}

// CurrentSettings resolves the settings through the installed policy source.
func CurrentSettings(ctx context.Context) (Settings, error) {
	policySource.RLock()
	fn := policySource.fn
	policySource.RUnlock()
	if fn == nil {
		return DefaultSettings(), nil
	}
	eff, err := fn(ctx)
	if err != nil {
		return Settings{}, err
	}
	return SettingsFrom(eff)
}
