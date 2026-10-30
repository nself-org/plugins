package selftest

// Purpose: the two operator keys of the tidy (P7-AUTO-20): tidy.free_target_mb, the free space
// `runs prune --tidy --apply` tries to reach, and tidy.cache_cap_mb, the byte cap it applies to
// the cache (0: the cache's own cap).
// Inputs:  the effective policy. Outputs: TidyConfig.
// Constraints: the keys are fixed: only the operator scopes set them, a repository cannot.

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// Policy keys of the tidy.
const (
	KeyFreeTarget = "tidy.free_target_mb"
	KeyCacheCap   = "tidy.cache_cap_mb"
)

// DefaultFreeTargetMB is the free space the tidy aims for when the operator names none.
const DefaultFreeTargetMB = 5120

const maxMB = 100_000_000

// TidyConfig is the resolved tidy.* policy.
type TidyConfig struct {
	FreeTargetMB int64
	CacheCapMB   int64 // 0: leave the cache's own cap
}

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when both keys are in the key set.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// Keys returns the spec of both keys.
func Keys() []policy.KeySpec {
	k := func(key string, def int64) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: "selftest", Kind: policy.TightenFixed, Default: def, Decode: func(raw json.RawMessage) (any, error) { return decodeMB(key, raw) }}
	}
	return []policy.KeySpec{k(KeyFreeTarget, DefaultFreeTargetMB), k(KeyCacheCap, 0)}
}

func decodeMB(key string, raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var n int64
	if err := dec.Decode(&n); err != nil || n < 0 || n > maxMB {
		return nil, fmt.Errorf("E672 %s: expected a whole number of megabytes from 0 to %d", key, maxMB)
	}
	return n, nil
}

// TidyConfigFrom reads the effective policy; a nil Effective is an error.
func TidyConfigFrom(eff *policy.Effective) (TidyConfig, error) {
	if eff == nil {
		return TidyConfig{}, fmt.Errorf("selftest: no effective policy")
	}
	c := TidyConfig{FreeTargetMB: DefaultFreeTargetMB}
	read := func(key string, into *int64) error {
		v, _, ok := eff.Value(key)
		if !ok {
			return nil
		}
		b, err := json.Marshal(v)
		if err == nil {
			err = json.Unmarshal(b, into)
		}
		if err != nil {
			return fmt.Errorf("E672 %s: effective value is not a number", key)
		}
		return nil
	}
	if err := read(KeyFreeTarget, &c.FreeTargetMB); err != nil {
		return TidyConfig{}, err
	}
	return c, read(KeyCacheCap, &c.CacheCapMB)
}
