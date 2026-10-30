package watch

// Purpose: the operator keys of the watcher (P7-AUTO-11): watch.branches, watch.max_branches,
//          watch.branch_days, watch.retry_max, watch.retry_gap, watch.status_context and
//          watch.drain_sec, and the typed view of them.
// Inputs:  *policy.Effective from the operator's resolver.
// Outputs: Keys (registered from init()), Defaults, ApplyPolicy.
// Constraints: operator scope only (TightenFixed, Spend), so a repository can never widen what a
//          personal watcher runs or posts; the registered Default is nil on purpose (a Fixed key with a
//          default could never be set, E670), so the defaults live in Defaults. A bad value is E804.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyBranches      = "watch.branches"
	KeyMaxBranches   = "watch.max_branches"
	KeyBranchDays    = "watch.branch_days"
	KeyRetryMax      = "watch.retry_max"
	KeyRetryGap      = "watch.retry_gap"
	KeyStatusContext = "watch.status_context"
	KeyDrainSec      = "watch.drain_sec"
	keyOwner         = "P7-AUTO-11"
)

// The defaults of the keys.
const (
	DefaultMaxBranches = 20
	DefaultBranchDays  = 3
	DefaultRetryMax    = 3
	DefaultRetryGap    = 10 * time.Minute
	DefaultDrain       = 120 * time.Second
	DefaultInterval    = 60 * time.Second
	DefaultStep        = time.Minute
	DefaultRunLimit    = time.Hour
)

var contextRe = regexp.MustCompile(`^nself-ci(/[a-z][a-z-]{0,30})?$`)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every key is in place.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	for _, k := range Keys() {
		if err := policy.RegisterKey(k); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// Keys returns the spec of every key this package owns.
func Keys() []policy.KeySpec {
	k := func(key string, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Decode: dec}
	}
	return []policy.KeySpec{
		k(KeyBranches, decodeBranches), k(KeyMaxBranches, decodeInt(KeyMaxBranches, 0, 100)),
		k(KeyBranchDays, decodeInt(KeyBranchDays, 1, 365)), k(KeyRetryMax, decodeInt(KeyRetryMax, 0, 10)),
		k(KeyRetryGap, decodeGap), k(KeyStatusContext, decodeContext), k(KeyDrainSec, decodeInt(KeyDrainSec, 0, 3600)),
	}
}

// Defaults is what an operator who set nothing gets.
func Defaults() Config {
	return Config{Interval: DefaultInterval, MaxBranches: DefaultMaxBranches, BranchDays: DefaultBranchDays,
		RetryMax: DefaultRetryMax, RetryGap: DefaultRetryGap, Drain: DefaultDrain, Step: DefaultStep, RunLimit: DefaultRunLimit}
}

// ApplyPolicy overlays the operator's watch.* values on c. A nil Effective leaves c unchanged.
func ApplyPolicy(c Config, eff *policy.Effective) (Config, error) {
	if eff == nil {
		return c, nil
	}
	num := func(key string, into *int) error {
		v, _, ok := eff.Value(key)
		if !ok {
			return nil
		}
		f, isNum := v.(float64)
		if n, isInt := v.(int); isInt {
			f, isNum = float64(n), true
		}
		if !isNum {
			return fail(key, "effective value %v is not a number", v)
		}
		*into = int(f)
		return nil
	}
	for key, into := range map[string]*int{KeyMaxBranches: &c.MaxBranches, KeyBranchDays: &c.BranchDays, KeyRetryMax: &c.RetryMax} {
		if err := num(key, into); err != nil {
			return c, err
		}
	}
	drain := int(c.Drain / time.Second)
	if err := num(KeyDrainSec, &drain); err != nil {
		return c, err
	}
	c.Drain = time.Duration(drain) * time.Second
	if v, _, ok := eff.Value(KeyRetryGap); ok {
		s, _ := v.(string)
		d, err := time.ParseDuration(s)
		if err != nil {
			return c, fail(KeyRetryGap, "effective value %v is not a duration", v)
		}
		c.RetryGap = d
	}
	if v, _, ok := eff.Value(KeyStatusContext); ok {
		c.StatusContext, _ = v.(string)
	}
	if v, _, ok := eff.Value(KeyBranches); ok {
		list, _ := v.([]any)
		c.Branches = nil
		for _, x := range list {
			if s, isStr := x.(string); isStr {
				c.Branches = append(c.Branches, s)
			}
		}
	}
	return c, nil
}

func fail(key, format string, a ...any) error {
	return &Error{Code: CodeConfig, Message: key + ": " + fmt.Sprintf(format, a...)}
}

func decodeOne(key string, raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	var v any
	if err := d.Decode(&v); err != nil || d.More() {
		return nil, fail(key, "expected exactly one JSON value")
	}
	return v, nil
}

func decodeInt(key string, lo, hi int) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeOne(key, raw)
		if err != nil {
			return nil, err
		}
		f, ok := v.(float64)
		if !ok || f != float64(int(f)) || int(f) < lo || int(f) > hi {
			return nil, fail(key, "expected a whole number from %d to %d", lo, hi)
		}
		return f, nil
	}
}

func decodeGap(raw json.RawMessage) (any, error) {
	v, err := decodeOne(KeyRetryGap, raw)
	if err != nil {
		return nil, err
	}
	s, _ := v.(string)
	d, perr := time.ParseDuration(s)
	if perr != nil || d < time.Minute || d > 24*time.Hour {
		return nil, fail(KeyRetryGap, "expected a duration such as 10m, from 1m to 24h")
	}
	return d.String(), nil
}

func decodeContext(raw json.RawMessage) (any, error) {
	v, err := decodeOne(KeyStatusContext, raw)
	if err != nil {
		return nil, err
	}
	s, _ := v.(string)
	if !contextRe.MatchString(s) {
		return nil, fail(KeyStatusContext, "expected nself-ci or nself-ci/<name>")
	}
	return s, nil
}

func decodeBranches(raw json.RawMessage) (any, error) {
	v, err := decodeOne(KeyBranches, raw)
	if err != nil {
		return nil, err
	}
	list, ok := v.([]any)
	if !ok || len(list) > 50 {
		return nil, fail(KeyBranches, "expected a list of at most 50 branch names or globs")
	}
	for _, x := range list {
		s, isStr := x.(string)
		if _, perr := path.Match(s, ""); !isStr || s == "" || perr != nil {
			return nil, fail(KeyBranches, "expected branch names or valid globs")
		}
	}
	return list, nil
}
