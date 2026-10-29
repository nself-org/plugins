package pools

// Purpose: the policy keys pools.allow and pools.dev (contract:ci.policy v1) and their reading from a
//          resolved policy.
// Inputs:  init() registers the keys with policy.RegisterKey; FromPolicy reads a trust.Effective.
// Outputs: Keys, FromPolicy (the two terms and the notes of values that did not apply).
// Constraints: pools.allow is a project key: the protected default branch's `.nself/ci.yaml` lists the
//          pools that may run the project, and the revision may only shrink that list (TightenSubset;
//          the loader ignores and reports a widening revision). pools.dev is an operator key: a value
//          from the branch, the revision, a pipeline or a job is ignored and reported here. Both keys
//          default to unset, which means every pool: no default value joins the policy digest.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// The policy keys this package owns.
const (
	KeyAllow = "pools.allow"
	KeyDev   = "pools.dev"
)

// Owner is the owner name of the pool keys.
const Owner = "pools"

func init() {
	for _, k := range Keys() {
		if err := policy.RegisterKey(k); err != nil {
			panic("pools: " + err.Error())
		}
	}
}

// Keys returns the specs of the pool keys.
func Keys() []policy.KeySpec {
	return []policy.KeySpec{
		{Key: KeyAllow, Owner: Owner, Kind: policy.TightenSubset, Decode: decodeIDs},
		{Key: KeyDev, Owner: Owner, Kind: policy.TightenSubset, Decode: decodeIDs},
	}
}

// decodeIDs accepts a JSON list of pool ids (implicit ids included).
func decodeIDs(raw json.RawMessage) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	var ids []string
	if err := d.Decode(&ids); err != nil || ids == nil {
		return nil, fmt.Errorf("expected a list of pool ids")
	}
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		if !ValidID(id) {
			return nil, fmt.Errorf("pool id %q is not valid", id)
		}
		out = append(out, id)
	}
	return out, nil
}

// List is an optional pool list: Set false means the key is unset (every pool).
type List struct {
	Set bool
	IDs []string
}

// Has reports whether id is in a set list.
func (l List) Has(id string) bool { return slices.Contains(l.IDs, id) }

// operatorSources are the scopes pools.dev may come from.
var operatorSources = map[trust.Source]bool{trust.SourceDefault: true, trust.SourceUser: true, trust.SourceTeam: true, trust.SourceProject: true, trust.SourceRun: true}

// FromPolicy reads the two pool terms from a resolved policy. notes lists every pool key value that did
// not apply: the loader's ignored entries for the pool keys and a pools.dev outside operator scope.
func FromPolicy(eff trust.Effective) (allow, dev List, notes []string) {
	allow = listValue(eff, KeyAllow)
	if v, src, ok := eff.Value(KeyDev); ok {
		if operatorSources[src] {
			dev = toList(v)
		} else {
			notes = append(notes, fmt.Sprintf("%s from %s ignored: an operator key", KeyDev, src))
		}
	}
	for _, ig := range eff.Ignored() {
		if ig.Key == KeyAllow || ig.Key == KeyDev {
			notes = append(notes, fmt.Sprintf("%s from %s ignored: %s", ig.Key, ig.Scope, ig.Reason))
		}
	}
	return allow, dev, notes
}

func listValue(eff trust.Effective, key string) List {
	v, _, ok := eff.Value(key)
	if !ok {
		return List{}
	}
	return toList(v)
}

func toList(v any) List {
	l := List{Set: true, IDs: []string{}}
	switch xs := v.(type) {
	case []string:
		l.IDs = append(l.IDs, xs...)
	case []any:
		for _, x := range xs {
			if s, ok := x.(string); ok {
				l.IDs = append(l.IDs, s)
			}
		}
	}
	slices.Sort(l.IDs)
	l.IDs = slices.Compact(l.IDs)
	return l
}
