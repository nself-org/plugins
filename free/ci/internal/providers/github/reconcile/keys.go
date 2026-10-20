package reconcile

// Purpose: the operator keys of the GitHub ingress mode (Epic D19, scenario audit P5) and the typed view
// of them: providers.github.ingress (poll or webhook), providers.github.poll_interval (the demand poll
// cadence) and providers.github.hook_url (which hook is the coordinator's, for redelivery).
// Inputs:  *policy.Effective from the operator's resolver (UsePolicy installs the source).
// Outputs: Settings; the keys register from init().
// Constraints: operator scope only. Every key is Spend: true and TightenFixed, so a repository can never
//          choose how the coordinator reaches GitHub or how often it polls; the registered Default is nil on
//          purpose (a non-nil default on a Fixed key could never be set, E670), so the defaults live in
//          DefaultSettings. A poll interval under MinPollInterval is raised to it (not refused). A bad
//          value is E672.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
)

// The policy keys.
const (
	KeyIngress      = "providers.github.ingress"
	KeyPollInterval = "providers.github.poll_interval"
	KeyHookURL      = "providers.github.hook_url"
	keyOwner        = "P7-HOST-17"
)

// The ingress modes.
const (
	IngressPoll    = "poll"
	IngressWebhook = "webhook"
)

// The poll interval bounds.
const (
	DefaultPollInterval = 30 * time.Second
	MinPollInterval     = 15 * time.Second
	maxPollInterval     = 24 * time.Hour
)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when everything is in place.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

// Keys returns the spec of every key this package owns.
func Keys() []policy.KeySpec {
	k := func(key string, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Decode: dec}
	}
	return []policy.KeySpec{k(KeyIngress, decodeIngress), k(KeyPollInterval, decodeInterval), k(KeyHookURL, decodeHookURL)}
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

func decodeIngress(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyIngress, raw)
	if err != nil {
		return nil, err
	}
	if s != IngressPoll && s != IngressWebhook {
		return nil, fail(KeyIngress, "expected %q or %q", IngressPoll, IngressWebhook)
	}
	return s, nil
}

// decodeInterval takes a Go duration text such as "30s" and raises anything under the minimum to it.
func decodeInterval(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyPollInterval, raw)
	if err != nil {
		return nil, err
	}
	d, perr := time.ParseDuration(s)
	if perr != nil || d <= 0 || d > maxPollInterval {
		return nil, fail(KeyPollInterval, "expected a duration such as 30s, above zero and at most 24h")
	}
	return max(d, MinPollInterval).String(), nil
}

func decodeHookURL(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyHookURL, raw)
	if err != nil {
		return nil, err
	}
	u, perr := url.Parse(s)
	if perr != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fail(KeyHookURL, "expected an https URL with a host and no user info, query or fragment")
	}
	return strings.TrimRight(s, "/"), nil
}

// Settings is the typed view of the keys, plus the runner label the demand poll matches.
type Settings struct {
	Ingress      string
	PollInterval time.Duration
	HookURL      string // "" means no hook is recognised: nothing is redelivered
	JITLabel     string
}

// DefaultSettings is what an operator who set nothing gets: outbound-only polling.
func DefaultSettings() Settings {
	return Settings{Ingress: IngressPoll, PollInterval: DefaultPollInterval, JITLabel: jit.DefaultLabel}
}

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("reconcile: no effective policy")
	}
	s := DefaultSettings()
	if v, _, ok := eff.Value(KeyIngress); ok {
		got, isStr := v.(string)
		if !isStr || (got != IngressPoll && got != IngressWebhook) {
			return Settings{}, fail(KeyIngress, "effective value %v is not poll or webhook", v)
		}
		s.Ingress = got
	}
	if v, _, ok := eff.Value(KeyPollInterval); ok {
		got, isStr := v.(string)
		d, err := time.ParseDuration(got)
		if !isStr || err != nil || d <= 0 {
			return Settings{}, fail(KeyPollInterval, "effective value %v is not a duration", v)
		}
		s.PollInterval = max(d, MinPollInterval)
	}
	if v, _, ok := eff.Value(KeyHookURL); ok {
		got, isStr := v.(string)
		if !isStr {
			return Settings{}, fail(KeyHookURL, "effective value %v is not a string", v)
		}
		s.HookURL = got
	}
	js, err := jit.SettingsFrom(eff)
	if err != nil {
		return Settings{}, err
	}
	s.JITLabel = js.Label
	return s, nil
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this once,
// next to runtime.UsePolicy and webhook.UsePolicy). With none installed the defaults apply and no
// repository has a per-repo secret. nil removes it.
func UsePolicy(fn func(context.Context) (*policy.Effective, error)) {
	policySource.Lock()
	defer policySource.Unlock()
	policySource.fn = fn
}

func currentPolicy(ctx context.Context) (*policy.Effective, error) {
	policySource.RLock()
	fn := policySource.fn
	policySource.RUnlock()
	if fn == nil {
		return nil, nil
	}
	return fn(ctx)
}
