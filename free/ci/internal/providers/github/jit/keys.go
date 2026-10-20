// Package jit is the registrar half of Epic D6 (P7-HOST-04): it registers GitHub just-in-time
// runners, tears them down, lists orphans and reads the runner-group restriction and the
// fork-approval policy, all over the provider REST funnel. It never launches a runner (P7-HOST-19).
package jit

// Purpose: the providers.github.jit.* policy keys and their typed view (Epic X1).
// Inputs:  *policy.Effective from the operator's resolver.
// Outputs: Settings; the keys register from init().
// Constraints: operator scope only. Every key is Spend: true and TightenFixed, so a repository can
//          never pick the label a runner registers with, the scope it registers at or the group a
//          tier lands in. The registered Default is nil on purpose (a non-nil default on a Fixed key
//          could never be set, E670); the defaults live in DefaultSettings. A bad value is E672.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyLabel  = "providers.github.jit.label"
	KeyScope  = "providers.github.jit.scope"
	KeyGroups = "providers.github.jit.groups"
	keyOwner  = "P7-HOST-04"
)

// The scopes a runner can register at.
const (
	ScopeRepo = "repo"
	ScopeOrg  = "org"
)

// DefaultLabel is the label a job asks for when the operator chose none.
const DefaultLabel = "nself"

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every key is registered.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() { registrationErrors = RegisterKeys() }

// RegisterKeys adds the three keys to the process-wide set and returns what failed.
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
	return []policy.KeySpec{k(KeyLabel, decodeLabel), k(KeyScope, decodeScope), k(KeyGroups, decodeGroups)}
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

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// reservedLabel reports a label the fixed set already carries, or one GitHub-hosted runners answer
// to: an operator who chose "ubuntu-latest" would pull every hosted job onto an nSelf node.
func reservedLabel(l string) bool {
	l = strings.ToLower(l)
	switch l {
	case "self-hosted", "linux", "windows", "macos", "x64", "x86", "arm", "arm64":
		return true
	}
	for _, p := range []string{"nself-jit-", "ubuntu-", "windows-", "macos-"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	return false
}

func decodeLabel(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyLabel, raw)
	if err != nil {
		return nil, err
	}
	if !labelPattern.MatchString(s) || reservedLabel(s) {
		return nil, fail(KeyLabel, "expected a plain label such as nself (not a GitHub-hosted or reserved label)")
	}
	return s, nil
}

func decodeScope(raw json.RawMessage) (any, error) {
	s, err := decodeString(KeyScope, raw)
	if err != nil {
		return nil, err
	}
	if s != ScopeRepo && s != ScopeOrg {
		return nil, fail(KeyScope, "expected repo or org")
	}
	return s, nil
}

var tierPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func decodeGroups(raw json.RawMessage) (any, error) {
	var m map[string]int64
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return nil, fail(KeyGroups, "expected an object mapping a tier name to a runner group id")
	}
	for tier, id := range m {
		if !tierPattern.MatchString(tier) || id <= 0 {
			return nil, fail(KeyGroups, "tier names are lowercase words and group ids are positive integers")
		}
	}
	return m, nil
}

// Settings is the typed view of the three keys.
type Settings struct {
	Label, Scope string
	Groups       map[string]int64
}

// DefaultSettings is what an operator who set nothing gets.
func DefaultSettings() Settings { return Settings{Label: DefaultLabel, Scope: ScopeRepo} }

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("jit: no effective policy")
	}
	s := DefaultSettings()
	if v, _, ok := eff.Value(KeyLabel); ok {
		got, isStr := v.(string)
		if !isStr {
			return Settings{}, fail(KeyLabel, "effective value %v is not a string", v)
		}
		s.Label = got
	}
	if v, _, ok := eff.Value(KeyScope); ok {
		got, isStr := v.(string)
		if !isStr {
			return Settings{}, fail(KeyScope, "effective value %v is not a string", v)
		}
		s.Scope = got
	}
	if v, _, ok := eff.Value(KeyGroups); ok {
		got, isMap := v.(map[string]int64)
		if !isMap {
			return Settings{}, fail(KeyGroups, "effective value %v is not a group map", v)
		}
		s.Groups = got
	}
	return s, nil
}
