package readiness

// Purpose: the readiness.* policy keys (AUTO D11) and their typed view.
// Inputs: *policy.Effective from the P7-TRUST-27 resolver.
// Outputs: Config. The keys register from init().
// Constraints: operator scope only. The keys carry Spend: true, which is how the policy
// engine keeps a key out of the protected branch, the revision, the pipeline and a run
// (readiness.cmd is a command the host executes, so a repository must never set it).
// Decoders refuse wrong shapes; a number that is not a whole percent or a positive
// decimal is an error. The policy digest has no floats, so max_load_per_core is stored
// as a decimal string ("1.5") whatever way the operator wrote it.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyMinMemMB       = "readiness.min_mem_mb"
	KeyMaxLoadPerCore = "readiness.max_load_per_core"
	KeyMinIdlePct     = "readiness.min_idle_pct"
	KeyMinDiskMB      = "readiness.min_disk_mb"
	KeyCmd            = "readiness.cmd"
	KeyYieldTo        = "readiness.yield_to"
)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every key is
// in the process-wide key set.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// Keys returns the spec of every readiness key.
func Keys() []policy.KeySpec {
	k := func(key string, def any, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: "readiness", Kind: policy.TightenAny, Spend: true, Default: def, Decode: dec}
	}
	return []policy.KeySpec{
		k(KeyMinMemMB, int64(DefaultMinMemMB), wholeNumber(KeyMinMemMB, math.MaxInt32)),
		k(KeyMaxLoadPerCore, formatDecimal(DefaultMaxLoadPerCore), decimal(KeyMaxLoadPerCore)),
		k(KeyMinIdlePct, int64(DefaultMinIdlePct), wholeNumber(KeyMinIdlePct, 100)),
		k(KeyMinDiskMB, int64(DefaultMinDiskMB), wholeNumber(KeyMinDiskMB, math.MaxInt32)),
		k(KeyCmd, nil, stringList(KeyCmd, true)),
		k(KeyYieldTo, nil, stringList(KeyYieldTo, false)),
	}
}

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeValue(key string, raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, fail(key, "malformed JSON")
	}
	if d.More() || v == nil {
		return nil, fail(key, "expected exactly one value")
	}
	return v, nil
}

// wholeNumber accepts 0..max; 0 switches the floor off.
func wholeNumber(key string, max int64) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeValue(key, raw)
		if err != nil {
			return nil, err
		}
		n, ok := v.(json.Number)
		if !ok {
			return nil, fail(key, "expected a whole number (0 switches the floor off)")
		}
		i, err := n.Int64()
		if err != nil || i < 0 || i > max {
			return nil, fail(key, "expected a whole number from 0 to %d, got %s", max, n)
		}
		return i, nil
	}
}

var plainDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// decimal accepts a plain number or numeric string from 0 to 1000 (0 switches the ceiling
// off; no exponent, sign or hex form) and stores the decimal string.
func decimal(key string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeValue(key, raw)
		if err != nil {
			return nil, err
		}
		var text string
		switch x := v.(type) {
		case json.Number:
			text = x.String()
		case string:
			text = strings.TrimSpace(x)
		default:
			return nil, fail(key, "expected a number such as 1.5")
		}
		f, err := strconv.ParseFloat(text, 64)
		if !plainDecimal.MatchString(text) || err != nil || f > 1000 {
			return nil, fail(key, "expected a plain number from 0 to 1000 such as 1.5 (0 switches the ceiling off), got %q", text)
		}
		return formatDecimal(f), nil
	}
}

func formatDecimal(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// stringList accepts a list of non-empty strings; a single argv list (cmd) must be non-empty.
func stringList(key string, nonEmpty bool) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeValue(key, raw)
		if err != nil {
			return nil, err
		}
		items, ok := v.([]any)
		if !ok || nonEmpty && len(items) == 0 {
			return nil, fail(key, "expected a list of strings")
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok || strings.TrimSpace(s) == "" {
				return nil, fail(key, "every entry must be a non-empty string")
			}
			out = append(out, s)
		}
		return out, nil
	}
}

// ConfigFrom reads the effective policy. A nil Effective is an error: silently using
// the defaults would drop the operator's overrides.
func ConfigFrom(eff *policy.Effective) (Config, error) {
	if eff == nil {
		return Config{}, fmt.Errorf("readiness: no effective policy")
	}
	c := DefaultConfig()
	var err error
	num := func(key string, into *int64) {
		if v, _, ok := eff.Value(key); ok && err == nil {
			n, isInt := asInt(v)
			if !isInt {
				err = fail(key, "effective value %v is not a whole number", v)
				return
			}
			*into = n
		}
	}
	num(KeyMinMemMB, &c.MinMemMB)
	num(KeyMinDiskMB, &c.MinDiskMB)
	var idle int64
	idle = int64(c.MinIdlePct)
	num(KeyMinIdlePct, &idle)
	c.MinIdlePct = float64(idle)
	if v, _, ok := eff.Value(KeyMaxLoadPerCore); ok && err == nil {
		s, isStr := v.(string)
		f, perr := strconv.ParseFloat(s, 64)
		if !isStr || perr != nil {
			err = fail(KeyMaxLoadPerCore, "effective value %v is not a decimal", v)
		} else {
			c.MaxLoadPerCore = f
		}
	}
	if err == nil {
		c.Cmd, err = listOf(eff, KeyCmd)
	}
	if err == nil {
		c.YieldTo, err = listOf(eff, KeyYieldTo)
	}
	return c, err
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func listOf(eff *policy.Effective, key string) ([]string, error) {
	v, _, ok := eff.Value(key)
	if !ok {
		return nil, nil
	}
	switch xs := v.(type) {
	case []string:
		return append([]string(nil), xs...), nil
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			s, isStr := x.(string)
			if !isStr {
				return nil, fail(key, "effective value %v is not a list of strings", v)
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fail(key, "effective value %v is not a list of strings", v)
}
