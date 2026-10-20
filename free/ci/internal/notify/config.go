package notify

// Purpose: the notify.* policy keys (operator scope) and their typed view.
//   notify.sinks          list of sink objects (Secret: true: URLs, secrets and argv may carry tokens)
//   notify.digest         "off" (default) or "daily"
//   notify.quiet          list of event kinds to leave out (failure, recovery, error_stuck, digest)
//   notify.allow_private  true lets a webhook dial a private address (default false)
// Inputs: *policy.Effective from the policy resolver. Outputs: Config; the keys register from init().
// Constraints: every key carries Spend: true, which keeps it out of the repository's reach
// (a sink URL or argv is something the host acts on). Decoders refuse unknown fields and
// wrong shapes, and their errors never echo a value (a value can be a secret). The policy
// digest sees normalised values, so a sink list is stored as plain maps.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeySinks        = "notify.sinks"
	KeyDigest       = "notify.digest"
	KeyQuiet        = "notify.quiet"
	KeyAllowPrivate = "notify.allow_private"
)

// Config is the resolved notify.* policy.
type Config struct {
	Sinks        []SinkSpec
	Digest       bool // notify.digest = daily
	Quiet        map[Kind]bool
	AllowPrivate bool
}

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every key is in the
// process-wide key set.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// Keys returns the spec of every notify key.
func Keys() []policy.KeySpec {
	k := func(key string, def any, secret bool, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: "notify", Kind: policy.TightenAny, Spend: true, Secret: secret, Default: def, Decode: dec}
	}
	return []policy.KeySpec{
		k(KeySinks, nil, true, decodeSinks),
		k(KeyDigest, "off", false, decodeDigest),
		k(KeyQuiet, nil, false, decodeQuiet),
		k(KeyAllowPrivate, false, false, decodeBool),
	}
}

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeStrict(key string, raw json.RawMessage, into any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return fail(key, "malformed value (unknown field or wrong type)")
	}
	if d.More() {
		return fail(key, "expected exactly one value")
	}
	return nil
}

func decodeSinks(raw json.RawMessage) (any, error) {
	var specs []SinkSpec
	if err := decodeStrict(KeySinks, raw, &specs); err != nil {
		return nil, err
	}
	if specs == nil {
		return nil, fail(KeySinks, "expected a list of sink objects")
	}
	seen := map[string]bool{}
	out := make([]any, 0, len(specs))
	for i := range specs {
		s := &specs[i]
		if s.Name == "" {
			s.Name = s.Type
			if seen[s.Name] {
				s.Name = fmt.Sprintf("%s-%d", s.Type, i+1)
			}
		}
		if seen[s.Name] || strings.ContainsAny(s.Name, " \t\r\n/\\") {
			return nil, fail(KeySinks, "sink %d: names must be unique and hold no spaces or slashes", i+1)
		}
		seen[s.Name] = true
		if _, err := New(*s, Deps{}); err != nil { // the factory validates; its errors never carry a value
			return nil, fail(KeySinks, "sink %d: %v", i+1, err)
		}
		m := map[string]any{"type": s.Type, "name": s.Name}
		if s.URL != "" {
			m["url"] = s.URL
		}
		if s.Secret != "" {
			m["secret"] = s.Secret
		}
		if len(s.Argv) != 0 {
			argv := make([]any, len(s.Argv))
			for j, a := range s.Argv {
				argv[j] = a
			}
			m["argv"] = argv
		}
		out = append(out, m)
	}
	return out, nil
}

func decodeDigest(raw json.RawMessage) (any, error) {
	var s string
	if err := decodeStrict(KeyDigest, raw, &s); err != nil || (s != "off" && s != "daily") {
		return nil, fail(KeyDigest, `expected "off" or "daily"`)
	}
	return s, nil
}

func decodeQuiet(raw json.RawMessage) (any, error) {
	var kinds []string
	if err := decodeStrict(KeyQuiet, raw, &kinds); err != nil || kinds == nil {
		return nil, fail(KeyQuiet, "expected a list of event kinds")
	}
	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		if !Kind(k).Valid() {
			return nil, fail(KeyQuiet, "unknown event kind (have %v)", Kinds())
		}
		out = append(out, k)
	}
	return out, nil
}

func decodeBool(raw json.RawMessage) (any, error) {
	var b bool
	if err := decodeStrict(KeyAllowPrivate, raw, &b); err != nil {
		return nil, fail(KeyAllowPrivate, "expected true or false")
	}
	return b, nil
}

// ConfigFrom reads the effective policy. A nil Effective is an error: silently using the
// defaults would drop the operator's overrides.
func ConfigFrom(eff *policy.Effective) (Config, error) {
	if eff == nil {
		return Config{}, fmt.Errorf("notify: no effective policy")
	}
	c := Config{Quiet: map[Kind]bool{}}
	if v, _, ok := eff.Value(KeySinks); ok {
		b, err := json.Marshal(v)
		if err == nil {
			err = json.Unmarshal(b, &c.Sinks)
		}
		if err != nil {
			return Config{}, fail(KeySinks, "effective value is not a sink list")
		}
	}
	if v, _, ok := eff.Value(KeyDigest); ok {
		c.Digest = v == "daily"
	}
	if v, _, ok := eff.Value(KeyQuiet); ok {
		b, err := json.Marshal(v)
		var kinds []Kind
		if err == nil {
			err = json.Unmarshal(b, &kinds)
		}
		if err != nil {
			return Config{}, fail(KeyQuiet, "effective value is not a list of kinds")
		}
		for _, k := range kinds {
			c.Quiet[k] = true
		}
	}
	if v, _, ok := eff.Value(KeyAllowPrivate); ok {
		c.AllowPrivate = v == true
	}
	return c, nil
}
