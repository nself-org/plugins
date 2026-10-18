package schedpolicy

// Purpose: strict decoders for the scheduling keys. Each returns the value in
// the form the merge compares and the typed views read: bool, string,
// []string or int64 (durations in milliseconds).
// Inputs: the raw JSON of one key. Outputs: the value or an E691 error with
// a JSON pointer. Constraints: unknown shapes, wrong types, floats, zero and
// negative numbers, empty names and duplicate list entries are all refused.

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"time"
)

type decoder = func(json.RawMessage) (any, error)

func decodeJSON(key string, raw json.RawMessage) (any, error) {
	if !json.Valid(raw) {
		return nil, fail(key, "malformed JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, fail(key, "%v", err)
	}
	if d.More() {
		return nil, fail(key, "trailing JSON value")
	}
	if v == nil {
		return nil, fail(key, "null value")
	}
	return v, nil
}

func boolOf(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeJSON(key, raw)
		if err != nil {
			return nil, err
		}
		if _, ok := v.(bool); !ok {
			return nil, fail(key, "expected a boolean")
		}
		return v, nil
	}
}

// stringOf accepts a non-empty string, optionally one of allowed.
func stringOf(key string, allowed []string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeJSON(key, raw)
		if err != nil {
			return nil, err
		}
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fail(key, "expected a non-empty string")
		}
		if allowed != nil && !slices.Contains(allowed, s) {
			return nil, fail(key, "unknown value %q (one of %v)", s, allowed)
		}
		return s, nil
	}
}

// listOf accepts a list of unique non-empty names. An empty list is refused
// when nonEmpty is set: the scheduler reads an empty allow list as "every
// provider", so writing [] would lift the very restriction an allow list sets.
func listOf(key string, nonEmpty bool) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeJSON(key, raw)
		if err != nil {
			return nil, err
		}
		items, ok := v.([]any)
		if !ok {
			return nil, fail(key, "expected a list of names")
		}
		if nonEmpty && len(items) == 0 {
			return nil, fail(key, "an empty list allows everything; list the allowed names or omit the key")
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, ok := item.(string)
			if !ok || s == "" {
				return nil, fail(key, "expected a list of non-empty names")
			}
			if slices.Contains(out, s) {
				return nil, fail(key, "duplicate entry %q", s)
			}
			out = append(out, s)
		}
		return out, nil
	}
}

// positiveInt accepts a whole number from 1 up to math.MaxInt64 - 1: the top
// value is the registered "no limit" default and cannot be written.
func positiveInt(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeJSON(key, raw)
		if err != nil {
			return nil, err
		}
		n, ok := v.(json.Number)
		if !ok {
			return nil, fail(key, "expected a positive whole number")
		}
		i, err := n.Int64()
		if err != nil || i < 1 || i == math.MaxInt64 {
			return nil, fail(key, "expected a positive whole number, got %s (omit the key for no limit)", n)
		}
		return i, nil
	}
}

// durationMs accepts a Go duration string of at least one millisecond and
// returns whole milliseconds.
func durationMs(key string) decoder {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeJSON(key, raw)
		if err != nil {
			return nil, err
		}
		s, ok := v.(string)
		if !ok {
			return nil, fail(key, `expected a duration string such as "10m"`)
		}
		ms, err := ParseDurationMs(key, s)
		if err != nil {
			return nil, err
		}
		return ms, nil
	}
}

// ParseDurationMs parses a Go duration of at least one millisecond into
// milliseconds; key names the field in the error.
func ParseDurationMs(key, s string) (int64, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fail(key, "%q is not a duration (use 30s, 10m, 2h)", s)
	}
	if d < time.Millisecond || d%time.Millisecond != 0 {
		return 0, fail(key, "duration %q must be a whole number of milliseconds, at least 1ms", s)
	}
	return int64(d / time.Millisecond), nil
}
