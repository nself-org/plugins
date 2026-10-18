package policy

// Purpose: the key registry (contract:ci.policy v1). TRUST registers trust.*,
// SCHED registers sched.*, economics.* and burst.* through RegisterKey.
// Inputs: KeySpec values from init() of the owning packages.
// Outputs: Keys, Find, and the trust.Registry the resolver merges with.
// Constraints: a refused spec is an error (E672), never a panic; a duplicate
// key or an overlapping `prefix.*` family is refused; a `prefix.*` key matches
// exactly one segment (trust.Registry.Find decides, so there is one rule).

import (
	"fmt"
	"slices"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Keyset is a set of registered keys. Production code uses the package-level
// Default through RegisterKey; tests build their own with NewKeyset.
type Keyset struct {
	mu    sync.RWMutex
	specs []KeySpec
}

// NewKeyset returns an empty key set.
func NewKeyset() *Keyset { return &Keyset{} }

// Default is the process-wide key set: init() of TRUST and SCHED fill it.
var Default = NewKeyset()

// RegisterKey adds a key to the process-wide set.
func RegisterKey(spec KeySpec) error { return Default.Register(spec) }

func refuse(format string, args ...any) error {
	return fmt.Errorf("E672 "+format, args...)
}

// Register validates and adds one key.
func (k *Keyset) Register(spec KeySpec) error {
	if err := checkSpec(spec); err != nil {
		return err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	next := append(slices.Clone(k.specs), spec)
	if _, err := trust.NewRegistry(next...); err != nil {
		return err
	}
	k.specs = next
	return nil
}

// Keys returns the registered keys in registration order.
func (k *Keyset) Keys() []KeySpec {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return slices.Clone(k.specs)
}

// Find returns the spec that governs key.
func (k *Keyset) Find(key string) (KeySpec, bool) {
	r, err := k.registry()
	if err != nil {
		return KeySpec{}, false
	}
	return r.Find(key)
}

func (k *Keyset) registry() (trust.Registry, error) {
	return trust.NewRegistry(k.Keys()...)
}

func checkSpec(s KeySpec) error {
	if s.Key == "" || s.Owner == "" {
		return refuse("key spec %q needs a key and an owner", s.Key)
	}
	if s.Decode == nil {
		return refuse("key %s has no Decode", s.Key)
	}
	switch s.Kind {
	case TightenLower, TightenHigher:
		if len(s.Order) == 0 {
			return refuse("key %s needs an Order", s.Key)
		}
		if s.Default != nil {
			d, ok := s.Default.(string)
			if !ok || !slices.Contains(s.Order, d) {
				return refuse("key %s default is not in its Order", s.Key)
			}
		}
	case TightenSubset, TightenSuperset:
		if s.Default != nil {
			if _, ok := stringList(s.Default); !ok {
				return refuse("key %s default is not a string list", s.Key)
			}
		}
	case TightenOnlyOn:
		if _, ok := s.Default.(bool); s.Default != nil && !ok {
			return refuse("key %s default is not a boolean", s.Key)
		}
	case TightenAny, TightenFixed:
	default:
		return refuse("key %s has an unknown Kind", s.Key)
	}
	return nil
}

func stringList(v any) ([]string, bool) {
	switch xs := v.(type) {
	case []string:
		return xs, true
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			s, ok := x.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}
