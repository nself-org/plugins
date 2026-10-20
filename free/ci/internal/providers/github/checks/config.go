package checks

// Purpose: the providers.github.app.* policy keys (operator scope) and their typed view.
//   providers.github.app.id               the GitHub App id (digits)
//   providers.github.app.installation_id  the installation of the App on the owner (digits)
//   providers.github.app.key_env          NAME of the environment variable that holds the PEM
// Inputs: *policy.Effective from the policy resolver. Outputs: Config; the keys register from init().
// Constraints: every key is Secret and Spend, and Fixed (an operator sets it, no lower scope may
// change it): a repository must never choose the App a signed token is minted for or the
// variable a key is read from. The value of key_env is a variable NAME, never a PEM. Decoders
// are strict and their errors never echo a value. The registered Default is nil on purpose (a
// Fixed key with a non-nil default could never be set, E670); an unset key means "no App".

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyAppID          = "providers.github.app.id"
	KeyInstallationID = "providers.github.app.installation_id"
	KeyKeyEnv         = "providers.github.app.key_env"
	keyOwner          = "P7-AUTO-22"
)

// Config is the resolved App configuration. The zero value means "no App".
type Config struct {
	AppID, InstallationID, KeyEnv string
}

// Enabled says whether every key is set: only then is a check run attempted.
func (c Config) Enabled() bool { return c.AppID != "" && c.InstallationID != "" && c.KeyEnv != "" }

// Partial says some but not all keys are set, which is an operator mistake worth a note.
func (c Config) Partial() bool {
	return !c.Enabled() && (c.AppID != "" || c.InstallationID != "" || c.KeyEnv != "")
}

var registrationErrors []error

// RegistrationErrors returns what init() could not register.
func RegistrationErrors() []error { return append([]error(nil), registrationErrors...) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// Keys returns the spec of every App key.
func Keys() []policy.KeySpec {
	k := func(key string, dec func(json.RawMessage) (any, error)) policy.KeySpec {
		return policy.KeySpec{Key: key, Owner: keyOwner, Kind: policy.TightenFixed, Spend: true, Secret: true, Decode: dec}
	}
	return []policy.KeySpec{k(KeyAppID, decodeID(KeyAppID)), k(KeyInstallationID, decodeID(KeyInstallationID)), k(KeyKeyEnv, decodeEnvName)}
}

func fail(key, what string) error { return fmt.Errorf("E672 %s: %s", key, what) }

func decodeOne(key string, raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil || d.More() || v == nil {
		return nil, fail(key, "expected exactly one value")
	}
	return v, nil
}

var digits = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// decodeID accepts a positive integer or a string of digits and stores the digits as a string.
func decodeID(key string) func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		v, err := decodeOne(key, raw)
		if err != nil {
			return nil, err
		}
		var s string
		switch x := v.(type) {
		case json.Number:
			s = x.String()
		case string:
			s = x
		}
		if _, perr := strconv.ParseUint(s, 10, 63); perr != nil || !digits.MatchString(s) {
			return nil, fail(key, "expected a positive whole number")
		}
		return s, nil
	}
}

// decodeEnvName accepts a variable name or "" (not set).
func decodeEnvName(raw json.RawMessage) (any, error) {
	v, err := decodeOne(KeyKeyEnv, raw)
	if err != nil {
		return nil, err
	}
	s, ok := v.(string)
	if !ok || s != "" && !envName.MatchString(s) {
		return nil, fail(KeyKeyEnv, "expected the NAME of an environment variable such as NSELF_CI_APP_KEY (never the key itself)")
	}
	return s, nil
}

// ConfigFrom reads the effective policy. A nil Effective is an error: silently reading "no App"
// would hide the operator's setting.
func ConfigFrom(eff *policy.Effective) (Config, error) {
	if eff == nil {
		return Config{}, fmt.Errorf("checks: no effective policy")
	}
	var c Config
	var err error
	str := func(key string, into *string) {
		if v, _, ok := eff.Value(key); ok && err == nil {
			got, isStr := v.(string)
			if !isStr {
				err = fail(key, "effective value is not a string")
				return
			}
			*into = got
		}
	}
	str(KeyAppID, &c.AppID)
	str(KeyInstallationID, &c.InstallationID)
	str(KeyKeyEnv, &c.KeyEnv)
	return c, err
}
