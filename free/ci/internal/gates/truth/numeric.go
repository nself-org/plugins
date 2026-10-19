package truth

// Purpose: the numeric-claim rule kind: a hand-typed count of plugins,
// commands, verbs, bundles or integrations in prose.
// Inputs:  one line of text and the compiled rules.
// Outputs: one hit per match of the rule's pattern.
// Constraints: Epic TRUTH D6. The pattern is RE2, matched case-insensitively
// (the loader prefixes (?i)); whether a hit is exempt (generated region, JSX
// expression, `truth: exempt` comment, exempt path or match) is decided by the
// scanner, not here; no network.

// patternHits returns every match of a claim rule's pattern in line.
func patternHits(r *Rule, line string) []hit {
	var out []hit
	for _, loc := range r.re.FindAllStringIndex(line, -1) {
		start := loc[0]
		// A pattern may start with a separator that keeps it off a longer token (a version, a path); the claim starts after it.
		for start < loc[1] && !isWordByte(line[start]) {
			start++
		}
		end := loc[1]
		for end > start && !isWordByte(line[end-1]) { // and a trailing one that keeps it off a longer token
			end--
		}
		if end > start {
			out = append(out, hit{Rule: r, Start: start, End: end, Text: line[start:end]})
		}
	}
	return out
}

// isWordByte reports whether b starts a word: an ASCII letter or digit, or the
// first byte of a multi-byte character.
func isWordByte(b byte) bool {
	return b >= 0x80 || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
