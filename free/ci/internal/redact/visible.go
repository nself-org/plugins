package redact

import (
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
	var out []byte
	var offsets []int
	for i := 0; i < len(b); {
		if b[i] == 0x1b {
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
		v := b[i : i+size]
		if folded && r != utf8.RuneError {
			v = []byte(string(unicode.ToLower(r)))
		}
		for j, c := range v {
			out = append(out, c)
			offsets = append(offsets, i+min(j, size-1))
		}
		i += size
	}
	return out, offsets
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
