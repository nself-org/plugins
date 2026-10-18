package redact

// Purpose: token shapes masked with no registered value: GitHub, GitLab, AWS,
// Stripe, Slack, Google and nSelf key prefixes, JWTs, Bearer credentials and
// PEM blocks. The prefix list is seeded from cli internal/observability/redact.go
// (apiKeyPrefixes, jwtRegex) and kept as data here because plugins cannot import
// cli/internal (ADR 0024 section 8).
// Inputs:  a buffer. Outputs: spans, the trailing bytes that may start a token,
// and the start of a PEM block that is not closed yet.
// Constraints: hand-written scans (no regexp), so cost is linear and a token is
// bounded by its rule's max run; PEM bodies are dropped as a stream by stream.go.

import (
	"bytes"
	"sort"
)

type shape struct {
	prefix   string
	set      *[256]bool
	min, max int
	boundary bool // the byte before the prefix must not be a letter or digit
	dots     int  // minimum '.' count in the run (JWT)
}

const (
	lower  = "abcdefghijklmnopqrstuvwxyz"
	upper  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits = "0123456789"
)

var (
	shapes  []shape
	shapeBy [256][]int
	pemBeg  = []byte("-----BEGIN ")
	pemEnd  = []byte("-----END ")
	dashes  = []byte("-----")
)

func charset(s string) *[256]bool {
	var t [256]bool
	for i := 0; i < len(s); i++ {
		t[s[i]] = true
	}
	return &t
}

func init() {
	defer func() {
		if len(shapes) > 64 {
			panic("redact: more than 64 shape rules; widen the skip table")
		}
	}()
	alnum := charset(lower + upper + digits)
	alnumUS := charset(lower + upper + digits + "_")
	alnumUSD := charset(lower + upper + digits + "_-")
	upperDigits := charset(upper + digits)
	jwt := charset(lower + upper + digits + "_-.")
	bearer := charset(lower + upper + digits + "._~+/=-")
	add := func(set *[256]bool, min, max int, prefixes ...string) {
		for _, p := range prefixes {
			shapes = append(shapes, shape{prefix: p, set: set, min: min, max: max})
		}
	}
	add(alnum, 16, 255, "ghp_", "gho_", "ghs_", "ghu_", "ghr_", "sk_live_", "rk_live_", "sk_test_", "rk_test_", "pk_live_", "pk_test_", "whsec_")
	add(alnumUS, 20, 255, "github_pat_")
	add(alnumUS, 16, 255, "nself_pro_", "nself_lic_")
	add(alnumUSD, 16, 255, "glpat-", "glrt-")
	add(upperDigits, 16, 16, "AKIA", "ASIA")
	add(alnumUSD, 35, 35, "AIza")
	add(alnumUSD, 10, 255, "xoxb-", "xoxa-", "xoxp-", "xoxr-", "xoxs-")
	add(bearer, 20, 4096, "Bearer ")
	shapes = append(shapes, shape{prefix: "sk-", set: alnumUSD, min: 20, max: 255, boundary: true})
	shapes = append(shapes, shape{prefix: "eyJ", set: jwt, min: 27, max: 8192, dots: 2})
	for i, s := range shapes {
		shapeBy[s.prefix[0]] = append(shapeBy[s.prefix[0]], i)
	}
	shapeBy['-'] = append(shapeBy['-'], -1) // PEM
}

// isAlnum reports an ASCII letter or digit.
func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// isLabel reports a byte allowed in a PEM label.
func isLabel(c byte) bool { return c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == ' ' }

// pemHeader parses "-----BEGIN LABEL-----" at b[i:]. ok means a full header
// ending at end; partial means b ends inside it (more bytes could complete it).
func pemHeader(b []byte, i int, pre []byte) (end int, ok, partial bool) {
	if !bytes.HasPrefix(b[i:], pre) {
		return 0, false, len(b)-i < len(pre) && bytes.HasPrefix(pre, b[i:])
	}
	k := i + len(pre)
	for k < len(b) && isLabel(b[k]) && k-i-len(pre) <= 40 {
		k++
	}
	if k == i+len(pre) && k < len(b) {
		return 0, false, false
	}
	rest := b[k:]
	if len(rest) >= 5 {
		if bytes.HasPrefix(rest, dashes) {
			return k + 5, true, false
		}
		return 0, false, false
	}
	return 0, false, bytes.HasPrefix(dashes, rest)
}

// findShapes scans b. prev is the byte before b (0 at the start of a stream).
// With final false a token touching the end of b is held back, not matched.
func findShapes(b []byte, prev byte, final bool, out []span) (spans []span, hold, pemAt int) {
	pemAt = -1
	var idx runIndex
	var skip [64]int // per rule: no start before this offset can succeed (its run ends at the same byte)
	keep := func(i int) {
		if len(b)-i > hold {
			hold = len(b) - i
		}
	}
	for i := 0; i < len(b); i++ {
		cands := shapeBy[b[i]]
		if len(cands) == 0 {
			continue
		}
		for _, ri := range cands {
			if ri < 0 {
				he, ok, partial := pemHeader(b, i, pemBeg)
				if partial && !final {
					keep(i)
				}
				if !ok {
					continue
				}
				if end, closed := pemClose(b, he); closed {
					out = append(out, span{i, end})
					i = end - 1
					break
				}
				if !final {
					pemAt = i
					return out, hold, pemAt
				}
				out = append(out, span{i, len(b)})
				return out, hold, -1
			}
			sh := &shapes[ri]
			if i < skip[ri] {
				continue
			}
			rest := b[i:]
			if len(rest) < len(sh.prefix) {
				if !final && string(rest) == sh.prefix[:len(rest)] {
					keep(i)
				}
				continue
			}
			if string(rest[:len(sh.prefix)]) != sh.prefix {
				continue
			}
			p := prev
			if i > 0 {
				p = b[i-1]
			}
			if sh.boundary && isAlnum(p) {
				continue
			}
			j, run, dots, partial := sh.runOf(b, i, &idx)
			if partial && !final {
				keep(i)
				continue
			}
			if sh.dots > 0 && run >= sh.min && dots >= sh.dots {
				if k := bytes.Index(b[i+len(sh.prefix):j], pemBeg[:len(pemBeg)-1]); k >= 0 { // the run stops at the header's space // a PEM header glued to the token
					j = i + len(sh.prefix) + k
					run, dots = j-i-len(sh.prefix), idx.dotsIn(i+len(sh.prefix), j)
				}
			}
			if run >= sh.min && dots >= sh.dots {
				out = append(out, span{i, j})
				i = j - 1
				break
			}
			if run < sh.max && j <= len(b) {
				skip[ri] = max(j-len(sh.prefix)+1, i+1) // a start whose prefix straddles j can still match
			}
		}
	}
	return out, hold, pemAt
}

// pemClose finds the end of the PEM block whose header ends at from.
func pemClose(b []byte, from int) (end int, closed bool) {
	for {
		idx := bytes.Index(b[from:], pemEnd)
		if idx < 0 {
			return 0, false
		}
		he, ok, _ := pemHeader(b, from+idx, pemEnd)
		if ok {
			return he, true
		}
		from += idx + len(pemEnd)
	}
}

// runOf measures the token run that follows the prefix at b[i:]. partial means
// the run touches the end of b, or ends inside what may become a PEM opener, so
// more bytes could extend it. A rule with a dot requirement (JWT) uses the run
// index, which keeps repeated failing candidates at O(log n) each.
func (sh *shape) runOf(b []byte, i int, idx *runIndex) (j, run, dots int, partial bool) {
	s := i + len(sh.prefix)
	if sh.dots > 0 {
		idx.build(b, sh.set)
		j = min(idx.firstBad(s, len(b)), s+sh.max)
		return j, j - s, idx.dotsIn(s, j), j == len(b) && j-s < sh.max
	}
	j = s
	for j < len(b) && sh.set[b[j]] && run < sh.max {
		if b[j] == '-' { // a PEM header glued to a token still opens its own block
			if rest := b[j:]; bytes.HasPrefix(rest, pemBeg) {
				break
			} else if len(rest) < len(pemBeg) && bytes.HasPrefix(pemBeg, rest) {
				return j, run, 0, true
			}
		}
		j++
		run++
	}
	return j, run, 0, j == len(b) && run < sh.max
}

// runIndex lists, once per scan (for the one rule with a dot requirement, JWT; the
// first charset it sees is kept), the offsets of bytes outside a rule's charset
// and of its dots, so a candidate's run end and dot count are binary searches.
type runIndex struct {
	bad, dots []int
	built     bool
}

func (x *runIndex) build(b []byte, set *[256]bool) {
	if x.built {
		return
	}
	x.built = true
	for i, c := range b {
		switch {
		case !set[c]:
			x.bad = append(x.bad, i)
		case c == '.':
			x.dots = append(x.dots, i)
		}
	}
}

// firstBad returns the first out-of-charset offset at or after from, or none.
func (x *runIndex) firstBad(from, none int) int {
	if k := sort.SearchInts(x.bad, from); k < len(x.bad) {
		return x.bad[k]
	}
	return none
}

// dotsIn counts the dots at offsets [from, to).
func (x *runIndex) dotsIn(from, to int) int {
	return sort.SearchInts(x.dots, to) - sort.SearchInts(x.dots, from)
}
