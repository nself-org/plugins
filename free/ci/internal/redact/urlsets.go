package redact

// Purpose: which reserved characters a URL encoder may leave raw, for one value.
// Inputs:  a value. Outputs: candidate "kept raw" character sets, most likely first.
// Constraints: the sets are per character, not per group, because real encoders
// differ on single characters (Go fragments keep "!" but not "'", Go userinfo
// keeps "$,;" but not ":"). A value with few distinct reserved characters gets
// every subset; a value with many gets the named encoder sets restricted to its
// characters. Named sets are never dropped; the optional subsets shrink as the
// value grows, so forms times length stays near urlBudget bytes per value.

import "math/bits"

// urlSpecials are the reserved characters that encoders disagree on.
const urlSpecials = "!'()*$&+=:@,;/?#"

// urlNamed are the safe sets of common encoders (each also holds the unreserved
// set): Go QueryEscape and PHP rawurlencode (none), Java URLEncoder, JS
// encodeURIComponent, Go PathEscape, Python quote, Go URL path, Go userinfo,
// Go fragment, JS encodeURI, Ruby URI escape.
var urlNamed = []string{"", "*", "!'()*", "$&+=:@", "/", "$&+,/:;=@", "$&+,;=", "!$&()*+,/:;=?@", "!#$&'()*+,/:;=?@", "!$&'()*+,/:;=?@"}

// urlBudget is the target for form bytes per value: sets * length * 4 hex/plus variants.
const urlBudget = 64 << 10

// urlSetsFor returns the kept-raw character sets to try for v.
func urlSetsFor(v []byte) []string {
	var present uint32
	for _, c := range v {
		for i := 0; i < len(urlSpecials); i++ {
			if urlSpecials[i] == c {
				present |= 1 << i
			}
		}
	}
	restrict := func(set string) string {
		out := ""
		for i := 0; i < len(set); i++ {
			for j := 0; j < len(urlSpecials); j++ {
				if urlSpecials[j] == set[i] && present&(1<<j) != 0 {
					out += set[i : i+1]
				}
			}
		}
		return out
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, n := range urlNamed { // every distinct named encoder set is kept, whatever the length
		add(restrict(n))
	}
	named := len(out)
	add(restrict(urlSpecials))
	if k := bits.OnesCount32(present); k <= 5 {
		for m := 0; m < 1<<k; m++ { // every subset of the characters present
			sub, idx := "", 0
			for j := 0; j < len(urlSpecials); j++ {
				if present&(1<<j) == 0 {
					continue
				}
				if m&(1<<idx) != 0 {
					sub += urlSpecials[j : j+1]
				}
				idx++
			}
			add(sub)
		}
	}
	if limit := named + max(urlBudget/(len(v)*4+1)-named, 0); len(out) > limit { // budget cuts only the optional subsets
		out = out[:limit]
	}
	return out
}
