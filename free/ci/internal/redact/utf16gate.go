package redact

// Purpose: a cheap, sound pre-check for the cache save scan (D-1515). The scan
// reads each window also as UTF-16 (both byte orders, both alignments), decodes
// it to UTF-8 and runs Scan on every decoded text. UTF16Candidate says, from the
// 16-bit units alone, when none of those four texts can hold anything Scan
// reports, so the decode and the four scans can be skipped.
// Inputs:  the registered patterns (via the snapshot) and a byte slice.
// Outputs: false only when every decoded view is guaranteed to scan clean.
// Constraints: the answer may be true when nothing matches (the caller then runs
// the full scan); it is never false when a view would be reported. Every Scan
// finding is one of two things, and the gate covers both:
//   1. a pattern of at least minLen bytes found in the visible form of the text.
//      The visible form keeps only the bytes of units that decode to a pattern
//      byte; everything else is skipped (invisible controls, whitespace, line
//      breaks, colour escapes) or is not a pattern byte at all. A match therefore
//      needs minLen units of class G (may be a pattern byte) with no unit of
//      class X (can neither be skipped nor be part of a match) between them.
//   2. the conservative mask for a partial pattern followed by a long tail of
//      skipped bytes (mappedScan), which needs a G unit and more than thr bytes
//      of G and T units up to the end of the text.
// The gate only reasons about ASCII patterns: one non-ASCII pattern byte
// disables it (UTF16Candidate then always says true).

import (
	"sync"
	"unicode"
)

const (
	clsX uint8 = iota // cannot match and cannot be skipped: ends a run
	clsT              // may be skipped (invisible, whitespace, escape body)
	clsG              // may decode to a pattern byte
)

// unitClass holds the class of every non-ASCII 16-bit unit. A surrogate is X:
// a pair decodes to a rune above U+FFFF (never skipped, never an ASCII letter
// once lower-cased) and an unpaired one to U+FFFD.
var unitClass struct {
	once sync.Once
	t    [1 << 16]uint8
}

func initUnitClass() {
	for u := 0x80; u < 1<<16; u++ {
		r := rune(u)
		switch {
		case u >= 0xD800 && u < 0xE000:
			unitClass.t[u] = clsX
		case unicode.ToLower(r) < 0x80: // folds to an ASCII byte (U+0130, U+212A)
			unitClass.t[u] = clsG
		case invisible(r) || unicode.IsSpace(r):
			unitClass.t[u] = clsT
		}
	}
}

// utf16Gate is the per-snapshot summary of the registered patterns.
type utf16Gate struct {
	none   bool // no pattern at all: nothing can match
	ascii  bool // every pattern byte is ASCII, so the gate can decide
	class  [128]uint8
	minLen int // shortest pattern, in bytes
	thr    int // the shortest hold the conservative mask needs, in decoded bytes
}

func newGate(autos ...*automaton) *utf16Gate {
	g := &utf16Gate{none: true, ascii: true}
	var alpha [256]bool
	for _, a := range autos {
		if a == nil {
			continue
		}
		for c, ok := range a.alpha {
			alpha[c] = alpha[c] || ok
		}
		if g.none || a.minLen < g.minLen {
			g.minLen = a.minLen
		}
		if t := maxSGRBytes * spanCap(a.maxLen); g.none || t < g.thr {
			g.thr = t
		}
		g.none = false
	}
	for c := 0x80; c < 256; c++ {
		if alpha[c] {
			g.ascii = false
		}
	}
	for c := 0; c < 128; c++ {
		r := rune(c)
		switch {
		case alpha[c] || c >= 'A' && c <= 'Z' && alpha[c+'a'-'A']:
			g.class[c] = clsG
		case invisible(r) || unicode.IsSpace(r):
			g.class[c] = clsT
		}
	}
	return g
}

func (s *snapshot) gate() *utf16Gate {
	s.gateOnce.Do(func() { s.g = newGate(s.a, s.b, s.c) })
	return s.g
}

// UTF16Candidate reports whether b, read as UTF-16 in either byte order from
// either alignment, could hold a registered value. False is a guarantee: Scan
// of each of the four decoded texts returns no match.
func (r *Redactor) UTF16Candidate(b []byte) bool {
	g := r.snapshot().gate()
	if g.none {
		return false
	}
	if !g.ascii {
		return true
	}
	unitClass.once.Do(initUnitClass)
	for _, big := range []bool{false, true} {
		for offset := 0; offset < 2; offset++ {
			if g.view(b, offset, big) {
				return true
			}
		}
	}
	return false
}

// view walks one reading of b. runLen and runG count the units and the G units
// since the last X unit; win is how many more units an escape sequence that
// began at a U+001B unit may still cover (colour sequences are at most
// maxSGRBytes bytes, each unit decodes to at least one byte).
func (g *utf16Gate) view(b []byte, offset int, big bool) bool {
	var runLen, runG, win int
	lo, hi := 0, 1
	if big {
		lo, hi = 1, 0
	}
	for i := offset; i+1 < len(b); i += 2 {
		u := uint16(b[i+lo]) | uint16(b[i+hi])<<8
		var c uint8
		if u < 0x80 {
			c = g.class[u]
		} else {
			c = unitClass.t[u]
		}
		if win > 0 {
			win--
			if c == clsX {
				c = clsT
			}
		}
		if u == 0x1b {
			win = maxSGRBytes
			if c == clsX {
				c = clsT
			}
		}
		switch c {
		case clsX:
			runLen, runG = 0, 0
		case clsT:
			runLen++
		default:
			runLen++
			runG++
			if runG >= g.minLen {
				return true
			}
		}
	}
	return runG > 0 && 3*runLen > g.thr // 3: the most bytes a skipped or pattern unit decodes to
}
