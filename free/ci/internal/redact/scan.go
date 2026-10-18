package redact

// Purpose: Scan reports where registered values sit inside a blob (cache
// artifacts, binary files). It is the authz.ValueScanner shape P7-CACHE-02 calls.
// Inputs:  a byte slice. Outputs: sorted, non-overlapping matches.
// Constraints: finds raw, encoded and base64 forms (line breaks inside base64
// are ignored); token shapes are a stream concern and are not reported here.

import "sort"

// Match is one occurrence: Len bytes starting at Offset.
type Match struct{ Offset, Len int }

// Scan returns the registered-value occurrences in blob.
func (r *Redactor) Scan(blob []byte) []Match {
	spans, _ := r.snapshot().values(blob)
	var out []Match
	for _, s := range mergeSpans(spans) {
		out = append(out, Match{Offset: s.s, Len: s.e - s.s})
	}
	return out
}

// mergeSpans sorts and joins overlapping or touching spans.
func mergeSpans(in []span) []span {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(i, j int) bool {
		if in[i].s != in[j].s {
			return in[i].s < in[j].s
		}
		return in[i].e > in[j].e
	})
	out := in[:1]
	for _, s := range in[1:] {
		last := &out[len(out)-1]
		if s.s <= last.e {
			if s.e > last.e {
				last.e = s.e
			}
			continue
		}
		out = append(out, s)
	}
	return out
}
