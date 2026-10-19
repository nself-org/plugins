package truth

// Purpose: find every rule hit on one line, with its position and text.
// Inputs:  a line and the compiled rules.
// Outputs: hits (rule, byte span, matched text, dead-host hash prefix).
// Constraints: spans are byte offsets into the line, so an exemption regex can
// cover them; a dead-host hit carries no text (the literal is never held); a
// retired term ending in `(s)` also matches its plural.

import "strings"

// hit is one rule broken at one place on a line.
type hit struct {
	Rule       *Rule
	Start, End int
	Text       string
	Hash       string
}

// lineHits returns every hit of every rule on the line, in rule order per kind.
func (c compiled) lineHits(line string) []hit {
	var out []hit
	for _, r := range c.literals {
		for from := 0; ; {
			i := strings.Index(line[from:], r.Literal)
			if i < 0 {
				break
			}
			s := from + i
			out = append(out, hit{Rule: r, Start: s, End: s + len(r.Literal), Text: r.Literal})
			from = s + len(r.Literal)
		}
	}
	if len(c.terms) > 0 {
		low := strings.ToLower(line)
		for i, t := range c.termsLow {
			for _, sp := range termSpans(low, t) {
				// Lower-casing can change the byte length (U+212A): then the offsets do not fit the line,
				// and the hit is taken to span the whole line, so an exemption must cover all of it.
				if len(low) != len(line) {
					out = append(out, hit{Rule: c.terms[i], Start: 0, End: len(line), Text: low[sp[0]:sp[1]]})
					continue
				}
				out = append(out, hit{Rule: c.terms[i], Start: sp[0], End: sp[1], Text: line[sp[0]:sp[1]]})
			}
		}
	}
	for _, h := range c.hostHits(line) {
		out = append(out, hit{Rule: h.Rule, Start: 0, End: len(line), Hash: h.Prefix})
	}
	for _, r := range c.claims {
		out = append(out, patternHits(r, line)...)
	}
	return out
}

// termSpans returns the spans of the whole-term matches of term in low. A term
// ending in `(s)` matches with and without the plural s.
func termSpans(low, term string) [][2]int {
	forms := []string{term}
	if strings.HasSuffix(term, "(s)") {
		base := strings.TrimSuffix(term, "(s)")
		forms = []string{base, base + "s"}
	}
	var out [][2]int
	for _, f := range forms {
		for from := 0; from < len(low); {
			i := strings.Index(low[from:], f)
			if i < 0 {
				break
			}
			start, end := from+i, from+i+len(f)
			if wholeTermAt(low, start, end) {
				out = append(out, [2]int{start, end})
			}
			from = start + 1
		}
	}
	return out
}
