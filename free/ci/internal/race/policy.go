// Package race runs sibling attempts of one job on distinct eligible runners
// (P7-AUTO-14, `--where all`). The first conclusive result stands; the other
// siblings are cancelled and never post.
//
// Purpose: this file owns the operator key sched.race.width: how many
// siblings one raced job may have.
// Inputs:  the resolved policy of the run (trust.Effective).
// Outputs: Width, the bound the selection applies.
// Constraints: TightenLower, default 2: a project or a run may only lower the
// width the operator allows; a value below 1 is refused at decode time.
package race

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// KeyWidth bounds the number of siblings of one raced job.
const KeyWidth = "sched.race.width"

// DefaultWidth is the registered default of KeyWidth.
const DefaultWidth = 2

// MaxWidth caps KeyWidth whatever the operator sets.
const MaxWidth = 16

// Owner is the owner recorded on the key this package registers.
const Owner = "sched"

var registrationErrors []error

// RegistrationErrors returns what init() could not register.
func RegistrationErrors() []error { return slices.Clone(registrationErrors) }

func init() {
	for _, spec := range Keys() {
		if err := policy.RegisterKey(spec); err != nil {
			registrationErrors = append(registrationErrors, err)
		}
	}
}

// Keys returns the spec of the race key.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{{Key: KeyWidth, Owner: Owner, Kind: policy.TightenLower, Default: int64(DefaultWidth), Decode: decodeWidth}}
}

// decodeWidth accepts a whole number from 1 to MaxWidth.
func decodeWidth(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, fmt.Errorf("%s: %v", KeyWidth, err)
	}
	if d.More() {
		return nil, fmt.Errorf("%s: trailing JSON value", KeyWidth)
	}
	n, ok := v.(json.Number)
	if !ok {
		return nil, fmt.Errorf("%s: expected a whole number of siblings", KeyWidth)
	}
	i, err := n.Int64()
	if err != nil || i < 1 || i > MaxWidth {
		return nil, fmt.Errorf("%s: expected a whole number from 1 to %d, got %s", KeyWidth, MaxWidth, n)
	}
	return i, nil
}

// Width is the effective sched.race.width of a resolved policy: the default
// when the key is unset or holds a value of the wrong type, never above MaxWidth.
func Width(e trust.Effective) int {
	v, _, ok := e.Value(KeyWidth)
	if !ok {
		return DefaultWidth
	}
	var n int64
	switch x := v.(type) {
	case int64:
		n = x
	case int:
		n = int64(x)
	case json.Number:
		var err error
		if n, err = x.Int64(); err != nil {
			return DefaultWidth
		}
	default:
		return DefaultWidth
	}
	return int(min(max(n, 1), MaxWidth))
}
