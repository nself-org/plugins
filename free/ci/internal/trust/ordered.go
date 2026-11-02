package trust

// Purpose: the TightenOrdered rule and the digest view of an Effective policy.
// Inputs:  two list values (old, next) and the key spec; an Effective.
// Outputs: orderedTighter (is next a legal tightening of old) and Effective.Snapshot.
// Constraints: pure. A list is tighter when every entry of next is in old and, unless
// old still comes from the registry default (SourceDefault), next keeps the relative
// order of old (a subsequence). The first scope to set the key may reorder the bare
// default, because that is the operator choosing the order; once any scope has set it,
// even to a value equal to the default, every later scope can only drop entries.

import "sort"

// orderedList returns v as an ordered string list (nil when it is not one).
func orderedList(v any) []string {
	switch xs := v.(type) {
	case []string:
		return append([]string(nil), xs...)
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			s, ok := x.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

// orderedTighter reports whether next may replace old for a TightenOrdered key.
// fromDefault is true when no scope has set the key (provenance, not value equality).
func orderedTighter(old, next any, fromDefault bool) bool {
	a, b := orderedList(old), orderedList(next)
	if a == nil || b == nil {
		return false
	}
	if !setSubset(a, b) {
		return false
	}
	if fromDefault {
		return true
	}
	i := 0
	for _, x := range a {
		if i < len(b) && b[i] == x {
			i++
		}
	}
	return i == len(b)
}

// setSubset reports whether every entry of next is in old.
func setSubset(old, next []string) bool {
	have := map[string]bool{}
	for _, x := range old {
		have[x] = true
	}
	for _, x := range next {
		if !have[x] {
			return false
		}
	}
	return true
}

// SnapshotEntry is one effective key in a Snapshot.
type SnapshotEntry struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Source Source `json:"source"`
}

// Snapshot is every effective key (sorted by key) and the hosted authorization
// flags: the whole policy input a consumer reads, for hashing. Effective keeps its
// fields unexported, so json.Marshal of an Effective is {}.
type Snapshot struct {
	Values        []SnapshotEntry `json:"values"`
	HostedPublic  bool            `json:"hosted_public"`
	HostedProject bool            `json:"hosted_project"`
}

// Snapshot returns the explicit view of e.
func (e Effective) Snapshot() Snapshot {
	s := Snapshot{Values: make([]SnapshotEntry, 0, len(e.values)), HostedPublic: e.hostedPublic, HostedProject: e.hostedProject}
	for k, v := range e.values {
		s.Values = append(s.Values, SnapshotEntry{Key: k, Value: v.value, Source: v.source})
	}
	sort.Slice(s.Values, func(i, j int) bool { return s.Values[i].Key < s.Values[j].Key })
	return s
}
