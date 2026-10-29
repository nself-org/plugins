// Package record enforces the pools permitted to produce a status of record.
package record

// Purpose: register pools.record and evaluate trusted pool membership.
// Inputs: resolved policy and coordinator-assigned runner facts.
// Outputs: a record decision, independent of development placement.
// Constraints: revisions only narrow; missing or malformed facts fail closed.

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/pools"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Key is the protected project's record-pool policy key.
const Key = "pools.record"

// Reason is the exclusion and publication refusal detail.
const Reason = "not_a_record_pool"

// Code is the publication refusal code.
const Code = "E809"

func init() {
	if err := policy.RegisterKey(Spec()); err != nil {
		panic(err)
	}
	model.Register(model.Code{ID: Code, Class: "policy", Summary: Reason, Fix: "Run commit CI on a pool listed in pools.record"})
}

// Spec returns the tighten-only project key, with an unset dynamic default.
func Spec() policy.KeySpec {
	return policy.KeySpec{Key: Key, Owner: pools.Owner, Kind: policy.TightenSubset, Decode: decode}
}

func decode(raw json.RawMessage) (any, error) {
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil || ids == nil {
		return nil, fmt.Errorf("expected a list of pool ids")
	}
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		if !pools.ValidID(id) {
			return nil, fmt.Errorf("invalid pool id %q", id)
		}
		values = append(values, id)
	}
	return values, nil
}

// FromPolicy reads the effective list; a malformed value is an empty set.
func FromPolicy(eff trust.Effective) pools.List {
	v, _, ok := eff.Value(Key)
	if !ok {
		return pools.List{}
	}
	l := pools.List{Set: true}
	switch xs := v.(type) {
	case []string:
		l.IDs = slices.Clone(xs)
	case []any:
		for _, x := range xs {
			id, ok := x.(string)
			if !ok || !pools.ValidID(id) {
				return pools.List{Set: true}
			}
			l.IDs = append(l.IDs, id)
		}
	}
	return l
}

// Allows reports whether r may contribute to a status of record. Unset means
// the operator's own nodes and hosted runners; TEAM membership needs a list.
func Allows(list pools.List, r pools.Runner) bool {
	if r.ID == "" {
		return false
	}
	switch r.Source {
	case pools.SourceLocal, pools.SourceRegistry, pools.SourceProvider:
		if !list.Set {
			return true
		}
	case pools.SourceCoordinator:
		if !list.Set {
			return false
		}
	default:
		return false
	}
	return slices.ContainsFunc(pools.Memberships(r), list.Has)
}

// Excluded returns record-pool reasons without changing other placement terms.
func Excluded(list pools.List, runners []pools.Runner) map[string][]model.PlacementReasonDetail {
	out := map[string][]model.PlacementReasonDetail{}
	for _, r := range runners {
		if !Allows(list, r) {
			out[r.ID] = []model.PlacementReasonDetail{{Code: string(model.PoolExcluded), Detail: Reason}}
		}
	}
	return out
}
