// Package runners is the runner half of Epic HOST D11 (P7-HOST-07): it creates a GitLab project runner
// for exactly one job on an nSelf node (POST /user/runners, a glrt- authentication token), hands the token
// to the pinned gitlab-runner image on stdin through the credential wrapper, pauses the runner when its
// job starts, and deletes it when the run closes. It reuses the provider-neutral launcher of
// internal/providers/runnerjob; this package adds the GitLab registrar, the demand handler for source
// gitlab and the minter. Fixtures only (DEF-003): no live GitLab call is made by any test.
package runners

// Purpose: the providers.gitlab.runner.* policy keys and their typed view (Epic D11, Epic X1).
// Inputs:  *policy.Effective from the operator's resolver (UsePolicy installs the source).
// Outputs: Settings{Tags, MaxPending}; the keys register from init().
// Constraints: operator scope only. Both keys are Spend: true and TightenFixed, so a repository can never
//          pick the tags nSelf runners answer to or raise the number of runners nSelf hardware carries for
//          it. The registered Default is nil on purpose (a non-nil default on a Fixed key could never be
//          set, E670); the defaults live in DefaultSettings. A bad value is E672. The tags always contain
//          the demand tag nself (the Job Hook source only offers jobs that carry it), never the reserved
//          nself-glr- prefix (that namespace is the per-runner unique tag), and are plain ASCII words.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/policy"
)

// The policy keys.
const (
	KeyTags       = "providers.gitlab.runner.tags"
	KeyMaxPending = "providers.gitlab.runner.max_pending"
	keyOwner      = "P7-HOST-07"
)

// Limits and defaults.
const (
	// DemandTag is the runner tag a job asks for to be offered to nSelf (the Job Hook source's
	// webhook.DemandTag; a test pins the two together).
	DemandTag = "nself"
	// DefaultMaxPending is the cap on live runner demands per project when the operator set none.
	DefaultMaxPending = 2
	// MaxPendingLimit is the largest max_pending the key accepts.
	MaxPendingLimit = 50
	maxTags         = 8
)

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

var registrationErrors []error

// RegistrationErrors returns what init() could not register; empty when every key is in place.
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
	return []policy.KeySpec{k(KeyTags, decodeTags), k(KeyMaxPending, decodeMaxPending)}
}

func fail(key, format string, a ...any) error {
	return fmt.Errorf("E672 %s: %s", key, fmt.Sprintf(format, a...))
}

func decodeTags(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	var tags []string
	if err := d.Decode(&tags); err != nil || d.More() || tags == nil {
		return nil, fail(KeyTags, "expected a JSON array of tag strings")
	}
	if err := checkTags(tags); err != nil {
		return nil, fail(KeyTags, "%v", err)
	}
	return tags, nil
}

// checkTags applies the tag rules shared by the key decoder and the registrar.
func checkTags(tags []string) error {
	if len(tags) == 0 || len(tags) > maxTags {
		return fmt.Errorf("expected 1 to %d tags", maxTags)
	}
	seen := map[string]bool{}
	for _, t := range tags {
		switch {
		case !tagPattern.MatchString(t):
			return fmt.Errorf("tag %q is not a plain word", t)
		case strings.HasPrefix(strings.ToLower(t), RunnerPrefix):
			return fmt.Errorf("tag %q is in the reserved %s namespace", t, RunnerPrefix)
		case seen[t]:
			return fmt.Errorf("tag %q is repeated", t)
		}
		seen[t] = true
	}
	if !seen[DemandTag] {
		return fmt.Errorf("the tags must include %s, the tag a job asks for", DemandTag)
	}
	return nil
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

// Settings is what the registrar and the handler need from the policy.
type Settings struct {
	Tags       []string
	MaxPending int
}

// DefaultSettings is what an operator who set nothing gets.
func DefaultSettings() Settings {
	return Settings{Tags: []string{DemandTag}, MaxPending: DefaultMaxPending}
}

// SettingsFrom reads the effective policy. A nil Effective is an error: silently using the defaults
// would drop the operator's overrides.
func SettingsFrom(eff *policy.Effective) (Settings, error) {
	if eff == nil {
		return Settings{}, fmt.Errorf("runners: no effective policy")
	}
	s := DefaultSettings()
	if v, _, ok := eff.Value(KeyTags); ok {
		tags, isList := v.([]string)
		if !isList || checkTags(tags) != nil {
			return Settings{}, fail(KeyTags, "effective value %v is not a tag list", v)
		}
		s.Tags = append([]string(nil), tags...)
	}
	if v, _, ok := eff.Value(KeyMaxPending); ok {
		n, isInt := v.(int)
		if !isInt || n < 1 || n > MaxPendingLimit {
			return Settings{}, fail(KeyMaxPending, "effective value %v is not a whole number from 1 to %d", v, MaxPendingLimit)
		}
		s.MaxPending = n
	}
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
