// Package usage is the GitHub enhanced-billing importer (P7-HOST-10, Epic D12). It reads the
// consumption GitHub reports for one account and writes the Actions minute lines into the SCHED-07
// usage ledger through economics.ReplaceUsage, so `nself ci policy usage` can show them as `known`.
// It never reports a remaining value and never keeps a ledger of its own.
package usage

// Purpose: the providers.github.billing_* policy keys and their typed view (Epic X1).
// Inputs:  *policy.Effective from the operator's resolver.
// Outputs: Settings; the keys register from init().
// Constraints: operator scope only. Both keys are Spend: true and TightenFixed, so a repository can
//          never switch the import on or point it at another account. The registered Default is nil
//          on purpose (a non-nil default on a Fixed key could never be set, E670); the defaults live
//          in DefaultSettings (import off, no account). A bad value is E672. The account carries its
//          kind ("user:<login>" or "org:<login>") because the two live at different endpoints and
//          guessing one would send the token to the wrong one.

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
	KeyBillingImport  = "providers.github.billing_import"
	KeyBillingAccount = "providers.github.billing_account"
	keyOwner          = "P7-HOST-10"
)

// The account kinds.
const (
	KindUser = "user"
	KindOrg  = "org"
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
	return []policy.KeySpec{k(KeyBillingImport, decodeImport), k(KeyBillingAccount, decodeAccount)}
}

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeOne(key string, raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	var v any
	if err := d.Decode(&v); err != nil || d.More() {
		return nil, fail(key, "expected exactly one JSON value")
	}
	return v, nil
}

func decodeImport(raw json.RawMessage) (any, error) {
	v, err := decodeOne(KeyBillingImport, raw)
	if err != nil {
		return nil, err
	}
	b, ok := v.(bool)
	if !ok {
		return nil, fail(KeyBillingImport, "expected true or false")
	}
	return b, nil
}

// loginPattern is a GitHub login: letters, digits, hyphen and underscore (enterprise shortcodes), at
// most 39 characters.
var loginPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,38}$`)

// Account is the billing account the importer reads.
type Account struct{ Kind, Login string }

// String is the policy form, "user:<login>" or "org:<login>".
func (a Account) String() string { return a.Kind + ":" + a.Login }

// ParseAccount reads the policy form of an account.
func ParseAccount(s string) (Account, error) {
	kind, login, ok := strings.Cut(s, ":")
	if !ok || (kind != KindUser && kind != KindOrg) || !loginPattern.MatchString(login) {
		return Account{}, fail(KeyBillingAccount, "expected user:<login> or org:<login>")
	}
	return Account{Kind: kind, Login: login}, nil
}

func decodeAccount(raw json.RawMessage) (any, error) {
	v, err := decodeOne(KeyBillingAccount, raw)
	if err != nil {
		return nil, err
	}
	s, ok := v.(string)
	if !ok {
		return nil, fail(KeyBillingAccount, "expected a string such as org:acme")
	}
	if _, err := ParseAccount(s); err != nil {
		return nil, err
	}
	return s, nil
}

// Settings is the typed view of the two keys.
type Settings struct {
	Import  bool
	Account string // "" until the operator names one
}

// DefaultSettings is what an operator who set nothing gets: no import.
func DefaultSettings() Settings { return Settings{} }

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("usage: no effective policy")
	}
	s := DefaultSettings()
	if v, _, ok := eff.Value(KeyBillingImport); ok {
		got, isBool := v.(bool)
		if !isBool {
			return Settings{}, fail(KeyBillingImport, "effective value %v is not a boolean", v)
		}
		s.Import = got
	}
	if v, _, ok := eff.Value(KeyBillingAccount); ok {
		got, isStr := v.(string)
		if !isStr {
			return Settings{}, fail(KeyBillingAccount, "effective value %v is not a string", v)
		}
		s.Account = got
	}
	return s, nil
}
