package redact

// Purpose: a frozen, test-only copy of the pre-D-1515 visible form and mapped
// scan (from the base commit of the PR), and of snapshot.values built on it. The
// differential tests use it as the reference, so a bug in appendVisible or
// mappedFind cannot hide by being shared with the reference. Do not edit it to
// follow changes in visible.go.

import (
	"unicode"
	"unicode/utf8"
)

// visible keeps original byte offsets, so a mask includes any inserted controls.
// folded also ignores whitespace and case for the raw/reversed value family.
func refVisible(b []byte, folded bool) ([]byte, []int) {
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

func refMappedScan(a *automaton, b []byte, folded bool) ([]span, int) {
	if a == nil {
		return nil, 0
	}
	v, offsets := refVisible(b, folded)
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

// refValues is snapshot.values as it was, over the frozen refMappedScan.
func refValues(s *snapshot, b []byte) (spans []span, hold int) {
	if s.a != nil {
		spans, hold = s.a.find(b, nil)
	}
	if s.b != nil {
		var h int
		spans, h = s.b.findSkip(b, spans)
		if h > hold {
			hold = h
		}
	}
	for _, scan := range []struct {
		a      *automaton
		folded bool
	}{{s.a, false}, {s.b, false}, {s.c, true}} {
		found, h := refMappedScan(scan.a, b, scan.folded)
		spans = append(spans, found...)
		hold = max(hold, h)
	}
	return spans, hold
}
