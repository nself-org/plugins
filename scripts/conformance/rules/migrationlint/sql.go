// sql.go: token helpers shared by the idempotency lint and the rename extractor (P7 D-0346).
package main

import "strings"

// w returns the upper-cased keyword at i, or "" when i is out of range or not a bare word.
func w(t []tok, i int) string {
	if i < 0 || i >= len(t) || t[i].K != kWord {
		return ""
	}
	return t[i].U
}

// has reports whether the words in seq start at t[i].
func has(t []tok, i int, seq ...string) bool {
	for k, s := range seq {
		if w(t, i+k) != s {
			return false
		}
	}
	return true
}

func isPunct(t []tok, i int, p string) bool {
	return i >= 0 && i < len(t) && t[i].K == kPunct && t[i].U == p
}

// ident returns the normalised name of the identifier token at i ("" when it is not one).
func ident(t []tok, i int) string {
	if i < 0 || i >= len(t) {
		return ""
	}
	switch t[i].K {
	case kQID:
		return t[i].U
	case kWord:
		return strings.ToLower(t[i].U)
	}
	return ""
}

// qname reads a possibly schema-qualified name at i: it returns schema ("" if unqualified), name and the next index.
func qname(t []tok, i int) (string, string, int) {
	a := ident(t, i)
	if a == "" {
		return "", "", i
	}
	if isPunct(t, i+1, ".") && ident(t, i+2) != "" {
		return a, ident(t, i+2), i + 3
	}
	return "", a, i + 1
}

// dotted renders a qualified name for output: "public." is dropped.
func dotted(schema, name string) string {
	if schema == "" || schema == "public" {
		return name
	}
	return schema + "." + name
}

// ine reports IF NOT EXISTS at i.
func ine(t []tok, i int) bool { return has(t, i, "IF", "NOT", "EXISTS") }

// ifExists looks for IF EXISTS in the first few tokens after i.
func ifExists(t []tok, i, span int) int {
	for k := i; k < i+span && k < len(t); k++ {
		if has(t, k, "IF", "EXISTS") {
			return k
		}
	}
	return -1
}

// contains reports whether the word sequence appears anywhere in t.
func contains(t []tok, seq ...string) bool {
	for i := range t {
		if has(t, i, seq...) {
			return true
		}
	}
	return false
}

// splitTop cuts t at commas that sit outside parentheses.
func splitTop(t []tok) [][]tok {
	var out [][]tok
	depth, start := 0, 0
	for i, x := range t {
		if x.K != kPunct {
			continue
		}
		switch x.U {
		case "(":
			depth++
		case ")":
			depth--
		case ",":
			if depth == 0 {
				out = append(out, t[start:i])
				start = i + 1
			}
		}
	}
	return append(out, t[start:])
}

// stripPL drops the plpgsql scaffolding at the start of a body statement (BEGIN, ELSE, LOOP, IF .. THEN, FOR .. LOOP,
// END IF ...) so the SQL command that follows can be classified. It returns nil for pure control statements.
func stripPL(t []tok) []tok {
	for len(t) > 0 {
		switch w(t, 0) {
		case "BEGIN", "ELSE", "LOOP", "DECLARE", "EXCEPTION":
			t = t[1:]
		case "IF", "ELSIF", "ELSEIF", "WHILE", "FOR", "FOREACH", "WHEN":
			depth, k := 0, 1
			for ; k < len(t); k++ {
				if isPunct(t, k, "(") {
					depth++
				} else if isPunct(t, k, ")") {
					depth--
				} else if depth == 0 && (w(t, k) == "THEN" || w(t, k) == "LOOP") {
					break
				}
			}
			if k >= len(t) {
				return nil
			}
			t = t[k+1:]
		case "END":
			return nil
		default:
			return t
		}
	}
	return nil
}
