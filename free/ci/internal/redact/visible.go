package redact

import (
	"sync"
	"unicode"
	"unicode/utf8"
)

// invisible controls cannot separate a secret into visually identical fragments.
func invisible(r rune) bool {
	return r >= 0x80 && r <= 0x9f || r == 0 || r == 0xad || r == 0x61c || r == 0x200b || r == 0x200c || r == 0x200d || r == 0x200e || r == 0x200f || r == 0x2060 || r == 0xfeff || r >= 0x2028 && r <= 0x202e || r >= 0x2066 && r <= 0x2069
}

// visible keeps original byte offsets, so a mask includes any inserted controls.
// folded also ignores whitespace and case for the raw/reversed value family.
func visible(b []byte, folded bool) ([]byte, []int) {
	var offsets []int
	out := appendVisible(nil, &offsets, b, folded)
	return out, offsets
}

// appendVisible appends the visible form of b to dst. When offsets is not nil
// it also records, for every appended byte, the original offset it came from.
// It is the one definition of the visible form: visible (with offsets) and the
// scan-only path (without, in a pooled buffer) cannot diverge.
func appendVisible(dst []byte, offsets *[]int, b []byte, folded bool) []byte {
	for i := 0; i < len(b); {
		c := b[i]
		if c < utf8.RuneSelf && c != 0x1b { // ASCII fast path: same outcome as the general case below
			i++
			if c == 0 || folded && (c == ' ' || c >= '\t' && c <= '\r') {
				continue
			}
			if folded && c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			dst = append(dst, c)
			if offsets != nil {
				*offsets = append(*offsets, i-1)
			}
			continue
		}
		if c == 0x1b {
			end, sgr, partial := escape(b, i)
			if partial {
				break
			}
			if sgr {
				i = end
				continue
			}
		}
		if !utf8.FullRune(b[i:]) {
			break
		}
		r, size := utf8.DecodeRune(b[i:])
		if size == 1 && b[i] >= 0x80 && b[i] <= 0x9f {
			i++
			continue
		}
		if invisible(r) || folded && unicode.IsSpace(r) {
			i += size
			continue
		}
		if folded && r != utf8.RuneError {
			n := len(dst)
			dst = utf8.AppendRune(dst, unicode.ToLower(r))
			if offsets != nil {
				for j := 0; j < len(dst)-n; j++ {
					*offsets = append(*offsets, i+min(j, size-1))
				}
			}
		} else {
			dst = append(dst, b[i:i+size]...)
			if offsets != nil {
				for j := 0; j < size; j++ {
					*offsets = append(*offsets, i+min(j, size-1))
				}
			}
		}
		i += size
	}
	return dst
}

// visBufs recycles the visible-form buffers of mappedFind. It holds *visBuf, so
// returning a buffer does not allocate a slice header.
type visBuf struct{ b []byte }

var visBufs sync.Pool

// mappedFind is mappedScan for callers that only need the spans (Scan). In the
// common case, nothing found and no partial match at the end, the visible form
// is built without the per-byte offset table and nothing is returned, which is
// exactly what mappedScan returns then. In every other case it is mappedScan.
func mappedFind(a *automaton, b []byte, folded bool) []span {
	if a == nil {
		return nil
	}
	vb, _ := visBufs.Get().(*visBuf)
	if vb == nil {
		vb = new(visBuf)
	}
	if cap(vb.b) < len(b) {
		vb.b = make([]byte, 0, len(b)+len(b)/8)
	}
	vb.b = appendVisible(vb.b[:0], nil, b, folded)
	matches, hold := a.findSkip(vb.b, nil)
	visBufs.Put(vb)
	if len(matches) == 0 && hold == 0 {
		return nil
	}
	found, _ := mappedScan(a, b, folded)
	return found
}

func mappedScan(a *automaton, b []byte, folded bool) ([]span, int) {
	if a == nil {
		return nil, 0
	}
	v, offsets := visible(b, folded)
	matches, hold := a.findSkip(v, nil)
	for i, m := range matches {
		matches[i] = span{offsets[m.s], offsets[m.e-1] + 1}
	}
	if hold > 0 {
		hold = len(b) - offsets[len(v)-hold]
		// A job may print unlimited invisible padding after a partial secret.
		// Mask that prefix conservatively rather than retaining unbounded output.
		if hold > maxSGRBytes*spanCap(a.maxLen) {
			matches = append(matches, span{len(b) - hold, len(b)})
			hold = 0
		}
	}
	return matches, hold
}
