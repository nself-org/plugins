// Package tiers is the tier order of the `auto` placement preset (P7-AUTO-05).
//
// Purpose: under `auto` a runner's tier (hosted, vps, lan, local; see
// sched.TierFor) and its rank inside the tier become a score bias, so a better
// tier wins when the scores are close while readiness, capacity, trust and
// cost still overrule the order. This package owns the policy keys
// sched.auto.order and sched.auto.step_sec and the World.Bias contributor.
// Inputs:  the World the builder made (Policy.Preset, Policy.Trust, runners).
// Outputs: World.Bias (tier index x sched.TierStepMs + rank, in ms; the
// tier_rank score component).
// Constraints: pure; only the `auto` preset is touched, every other preset's
// world stays byte-identical; the bias is a score term, never a rank-only
// shortcut past eligibility or scoring; no spend path: hosted runners are in
// the world only when their provider emitted them and the eligibility rules
// (providers.allow, paid and budget checks) still decide.
package tiers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/world"
)

// Name is the world contributor name.
const Name = "tiers"

// Owner is the owner recorded on the keys this package registers.
const Owner = "sched"

// KeyOrder is the tier order key: a subset of DefaultOrder, best tier first.
const KeyOrder = "sched.auto.order"

// KeyStepSec is the bounded wait for a busy better tier, in seconds.
const KeyStepSec = sched.KeyAutoStepSec

// DefaultOrder is the registered default of KeyOrder.
func DefaultOrder() []string {
	return []string{sched.TierHosted, sched.TierVPS, sched.TierLAN, sched.TierLocal}
}

var registrationErrors []error

// RegistrationErrors returns what init() could not register.
func RegistrationErrors() []error { return slices.Clone(registrationErrors) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
	if err := world.Register(world.Contributor{Name: Name, Field: "Bias", Fn: contribute}); err != nil {
		registrationErrors = append(registrationErrors, err)
	}
}

// Keys returns the specs of the two auto keys.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{
		{Key: KeyOrder, Owner: Owner, Kind: policy.TightenSubset, Default: DefaultOrder(), Decode: decodeOrder},
		{Key: KeyStepSec, Owner: Owner, Kind: policy.TightenLower, Default: sched.DefaultAutoStepSec, Decode: decodeStep},
	}
}

func decodeJSON(key string, raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: %v", key, err)
	}
	if d.More() {
		return nil, fmt.Errorf("%s: trailing JSON value", key)
	}
	return v, nil
}

// decodeOrder accepts a non-empty list of unique tier names.
func decodeOrder(raw json.RawMessage) (any, error) {
	v, err := decodeJSON(KeyOrder, raw)
	if err != nil {
		return nil, err
	}
	items, ok := v.([]any)
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("%s: expected a non-empty list of tiers %v", KeyOrder, DefaultOrder())
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok || !slices.Contains(DefaultOrder(), s) {
			return nil, fmt.Errorf("%s: unknown tier %v (one of %v)", KeyOrder, item, DefaultOrder())
		}
		if slices.Contains(out, s) {
			return nil, fmt.Errorf("%s: duplicate tier %q", KeyOrder, s)
		}
		out = append(out, s)
	}
	return out, nil
}

// decodeStep accepts a whole number of seconds, 0 (never wait) or more.
func decodeStep(raw json.RawMessage) (any, error) {
	v, err := decodeJSON(KeyStepSec, raw)
	if err != nil {
		return nil, err
	}
	n, ok := v.(json.Number)
	if !ok {
		return nil, fmt.Errorf("%s: expected a whole number of seconds", KeyStepSec)
	}
	i, err := n.Int64()
	if err != nil || i < 0 {
		return nil, fmt.Errorf("%s: expected a whole number of seconds >= 0, got %s", KeyStepSec, n)
	}
	return i, nil
}
