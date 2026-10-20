// Package demand is the GitHub half of the forge-runner launcher (Epic HOST D6, P7-HOST-19): the
// DemandHandler registered for source github, the one policy key it owns, and the Minter that adapts the
// JIT registrar (P7-HOST-04) to the provider-neutral runnerjob package.
package demand

// Purpose: the providers.github.jit.max_pending policy key and the typed settings the handler reads.
// Inputs:  *policy.Effective from the operator's resolver (UsePolicy installs the source).
// Outputs: Settings{Label, MaxPending}; the key registers from init().
// Constraints: operator scope only: the key is Spend: true and TightenFixed, so a repository can never
//          raise the number of runners nSelf hardware carries for it. The registered Default is nil on
//          purpose (a non-nil default on a Fixed key could never be set, E670); the default lives in
//          DefaultSettings. A value outside 1..MaxPendingLimit is E672. The jit label comes from the
//          jit package's own key, read through jit.SettingsFrom.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers/github/jit"
)

// KeyMaxPending is the cap on live runner demands per repository.
const (
	KeyMaxPending = "providers.github.jit.max_pending"
	keyOwner      = "P7-HOST-19"
	// DefaultMaxPending is the cap when the operator set none.
	DefaultMaxPending = 2
	// MaxPendingLimit is the largest value the key accepts.
	MaxPendingLimit = 50
)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when the key is in place.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	if err := policy.RegisterKey(Key()); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
}

// Key returns the spec of the key this package owns.
func Key() policy.KeySpec {
	return policy.KeySpec{Key: KeyMaxPending, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Decode: decodeMaxPending}
}

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeMaxPending(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil || d.More() {
		return nil, fail(KeyMaxPending, "expected exactly one JSON value")
	}
	n, ok := v.(json.Number)
	if !ok {
		return nil, fail(KeyMaxPending, "expected a number")
	}
	i, err := n.Int64()
	if err != nil || i < 1 || i > MaxPendingLimit {
		return nil, fail(KeyMaxPending, "expected a whole number from 1 to %d", MaxPendingLimit)
	}
	return int(i), nil
}

// Settings is what the handler needs from the policy.
type Settings struct {
	Label      string
	MaxPending int
}

// DefaultSettings is what an operator who set nothing gets.
func DefaultSettings() Settings {
	return Settings{Label: jit.DefaultLabel, MaxPending: DefaultMaxPending}
}

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("demand: no effective policy")
	}
	s := DefaultSettings()
	if v, _, ok := eff.Value(KeyMaxPending); ok {
		n, isInt := v.(int)
		if !isInt || n < 1 || n > MaxPendingLimit {
			return Settings{}, fail(KeyMaxPending, "effective value %v is not a whole number from 1 to %d", v, MaxPendingLimit)
		}
		s.MaxPending = n
	}
	js, err := jit.SettingsFrom(eff)
	if err != nil {
		return Settings{}, err
	}
	s.Label = js.Label
	return s, nil
}

var policySource struct {
	sync.RWMutex
	fn func(context.Context) (*policy.Effective, error)
}

// UsePolicy installs the source of the operator's effective policy (the coordinator start does this once,
// next to runtime.UsePolicy). With none installed the defaults apply. nil removes it.
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

// CurrentSettings resolves the settings through the installed policy source.
func CurrentSettings(ctx context.Context) (Settings, error) {
	eff, err := currentPolicy(ctx)
	if err != nil {
		return Settings{}, err
	}
	if eff == nil {
		return DefaultSettings(), nil
	}
	return SettingsFrom(eff)
}
