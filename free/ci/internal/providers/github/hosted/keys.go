// Package hosted is the GitHub-hosted dispatch provider (P7-HOST-03, Epic D5): it runs one
// attempt as a workflow_dispatch run of the generated nself-ci-dispatch workflow on a standard
// GitHub-hosted runner, follows the run through the provider REST funnel and accepts its evidence
// only through internal/providers/hostedev.
package hosted

// Purpose: the providers.github.untrusted_ref and providers.github.agent_source policy keys and
//          their typed view (Epic X1).
// Inputs:  *policy.Effective from the operator's resolver (UsePolicy installs the source).
// Outputs: Settings; the keys register from init().
// Constraints: operator scope only. Both keys are Spend: true and TightenFixed, so a repository can
//          never choose the ref untrusted work is dispatched on or where the agent comes from. The
//          registered Default is nil on purpose (a non-nil default on a Fixed key could never be set,
//          E670); the defaults live in DefaultSettings. A bad value is E672. The ref name is checked
//          like a git branch name and may not start with refs/ or be the empty string.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyUntrustedRef = "providers.github.untrusted_ref"
	KeyAgentSource  = "providers.github.agent_source"
	keyOwner        = "P7-HOST-03"
)

// The agent sources and the default ref.
const (
	AgentRelease      = "release"
	AgentSource       = "source"
	DefaultUntrustRef = "nself-ci/untrusted"
)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when both keys are registered.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() { registrationErrors = RegisterKeys() }

// RegisterKeys adds the two keys to the process-wide set and returns what failed.
func RegisterKeys() []error {
	var errs []error
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// Keys returns the spec of every key this package owns.
func Keys() []policy.KeySpec {
	k := func(key string, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Decode: dec}
	}
	return []policy.KeySpec{k(KeyUntrustedRef, decodeRef), k(KeyAgentSource, decodeAgentSource)}
}

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeString(key string, raw json.RawMessage) (string, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	var v any
	if err := d.Decode(&v); err != nil || d.More() {
		return "", fail(key, "expected exactly one JSON value")
	}
	s, ok := v.(string)
	if !ok {
		return "", fail(key, "expected a string")
	}
	return s, nil
}

var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)

// ValidRef reports whether s is a plain branch name an operator may pick for untrusted work.
func ValidRef(s string) bool {
	return refPattern.MatchString(s) && !strings.HasPrefix(s, "refs/") && !strings.Contains(s, "..") &&
		!strings.Contains(s, "//") && !strings.HasSuffix(s, "/") && !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, ".lock")
}

func decodeRef(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyUntrustedRef, raw)
	if err != nil {
		return nil, err
	}
	if !ValidRef(s) {
		return nil, fail(KeyUntrustedRef, "expected a plain branch name such as %s", DefaultUntrustRef)
	}
	return s, nil
}

func decodeAgentSource(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyAgentSource, raw)
	if err != nil {
		return nil, err
	}
	if s != AgentRelease && s != AgentSource {
		return nil, fail(KeyAgentSource, "expected release or source")
	}
	return s, nil
}

// Settings is the typed view of the two keys.
type Settings struct{ UntrustedRef, AgentSource string }

// DefaultSettings is what an operator who set nothing gets.
func DefaultSettings() Settings {
	return Settings{UntrustedRef: DefaultUntrustRef, AgentSource: AgentSource}
}

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("hosted: no effective policy")
	}
	s := DefaultSettings()
	for key, into := range map[string]*string{KeyUntrustedRef: &s.UntrustedRef, KeyAgentSource: &s.AgentSource} {
		if v, _, ok := eff.Value(key); ok {
			got, isStr := v.(string)
			if !isStr {
				return Settings{}, fail(key, "effective value %v is not a string", v)
			}
			*into = got
		}
	}
	if !ValidRef(s.UntrustedRef) || s.AgentSource != AgentRelease && s.AgentSource != AgentSource {
		return Settings{}, fail(KeyUntrustedRef, "effective value is not valid")
	}
	return s, nil
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this
// once, next to runtime.UsePolicy). With none installed the defaults apply. Passing nil removes it.
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
