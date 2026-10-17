// renames.go: extract the table renames a migration file performs (P7 D-0346), so the legacy-renames fixture of a plugin
// can be required to match them. Literal ALTER TABLE ... RENAME TO statements (top level or inside a DO body) give one
// pair each. A DO body that renames tables through EXECUTE format('ALTER TABLE ... RENAME TO ...') takes its pairs from
// the 2-tuples of strings in that body's (VALUES ...) list. Any other RENAME TO is unrecognised and fails closed.
package main

import (
	"fmt"
	"sort"
	"strings"
)

// renames returns the sorted unique "old new" pairs of src, and the problems that make the result unreliable.
func renames(src string) ([]string, []viol) {
	seen := map[string]bool{}
	var errs []viol
	var lit func(st []tok)
	lit = func(st []tok) {
		if len(st) == 0 {
			return
		}
		if w(st, 0) == "ALTER" && w(st, 1) == "TABLE" {
			i := 2
			if has(st, i, "IF", "EXISTS") {
				i += 2
			}
			if w(st, i) == "ONLY" {
				i++
			}
			s, n, j := qname(st, i)
			if isPunct(st, j, "*") {
				j++
			}
			if has(st, j, "RENAME", "TO") {
				if nn := ident(st, j+2); n != "" && nn != "" {
					seen[dotted(s, n)+" "+nn] = true
				} else {
					errs = append(errs, viol{st[0].Line, "cannot read the table names of this RENAME TO"})
				}
			} else if contains(st, "RENAME", "TO") {
				errs = append(errs, viol{st[0].Line, "unrecognised RENAME TO in an ALTER TABLE statement"})
			}
			return
		}
		if contains(st, "RENAME", "TO") {
			switch w(st, 1) {
			case "INDEX", "SEQUENCE", "VIEW", "MATERIALIZED":
				if w(st, 0) == "ALTER" {
					return
				}
			}
			errs = append(errs, viol{st[0].Line, "unrecognised RENAME TO (not a table, index, sequence or view rename)"})
		}
	}
	// body scans one dollar-quoted (or DO string) body: literal renames, strings that rename dynamically, and nested bodies.
	var body func(toks []tok)
	body = func(toks []tok) {
		for i, x := range toks {
			if x.K != kStr && x.K != kBody {
				continue
			}
			switch renameString(x.U) {
			case "table":
				ps, ok := boundPairs(toks, i)
				if !ok {
					errs = append(errs, viol{x.Line, "dynamic table rename with no (VALUES ('old', 'new'), ...) list bound to the format arguments to read the pairs from: use FOR r IN SELECT o, n FROM (VALUES ...) AS v(o, n) LOOP EXECUTE format('ALTER TABLE ... RENAME TO ...', r.o, r.n)"})
				}
				for _, p := range ps {
					seen[p] = true
				}
			case "bad":
				errs = append(errs, viol{x.Line, "unrecognised RENAME TO inside a string or body"})
			case "":
				if x.K == kBody {
					body(lex(x.U, x.BodyLine))
				}
			}
		}
		for _, b := range split(toks) {
			lit(stripPL(b))
		}
	}
	for _, st := range split(lexFile(src)) {
		lit(st)
		if w(st, 0) == "DO" {
			if b, ok := doBody(st); ok {
				body(b)
			}
			continue
		}
		fn := (w(st, 0) == "CREATE" && (contains(st[:min(4, len(st))], "FUNCTION") || contains(st[:min(4, len(st))], "PROCEDURE")))
		for _, x := range st {
			if x.K == kBody || (fn && x.K == kStr) {
				body(lex(x.U, x.Line))
			}
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, errs
}

// renameString classifies a string literal: "" (no RENAME TO), "table" (ALTER TABLE ... RENAME TO), "ok" (index,
// sequence or view rename) or "bad" (anything else). The string is lexed, so comments and odd spacing between the words
// do not hide a rename (D-0619 F6); a RENAME that is not followed by TO, COLUMN, CONSTRAINT, VALUE or ATTRIBUTE is bad.
func renameString(s string) string {
	t := lex(s, 1)
	found := false
	for i := range t {
		if w(t, i) != "RENAME" {
			continue
		}
		switch w(t, i+1) {
		case "TO":
			found = true
		case "COLUMN", "CONSTRAINT", "VALUE", "ATTRIBUTE":
		default:
			return "bad"
		}
	}
	if !found {
		return ""
	}
	switch {
	case has(t, 0, "ALTER", "TABLE"):
		return "table"
	case has(t, 0, "ALTER", "INDEX"), has(t, 0, "ALTER", "SEQUENCE"), has(t, 0, "ALTER", "VIEW"), has(t, 0, "ALTER", "MATERIALIZED", "VIEW"):
		return "ok"
	}
	return "bad"
}

// boundPairs reads the rename pairs of the dynamic table rename whose string is toks[at] (D-0619 F7). The string has to be
// the first argument of format(..) with two arguments r.old_col, r.new_col, where r is the variable of the closest
// `FOR r IN` before a (VALUES ('old', 'new'), ...) AS alias(old_col, new_col) list and the columns are that alias's first
// and second column. Every tuple of the list has to be a pair of strings. Anything else is not bound (ok false).
func boundPairs(toks []tok, at int) ([]string, bool) {
	if at < 2 || !isPunct(toks, at-1, "(") || w(toks, at-2) != "FORMAT" {
		return nil, false
	}
	// , 'old' , 'new' )  or  , rec . col , rec . col )
	k := at + 1
	if isPunct(toks, k, ",") && k+4 < len(toks) && toks[k+1].K == kStr && isPunct(toks, k+2, ",") && toks[k+3].K == kStr && isPunct(toks, k+4, ")") {
		return []string{strings.TrimPrefix(toks[k+1].U, "public.") + " " + toks[k+3].U}, true
	}
	var arg [2][2]string
	for n := 0; n < 2; n++ {
		if !isPunct(toks, k, ",") || ident(toks, k+1) == "" || !isPunct(toks, k+2, ".") || ident(toks, k+3) == "" {
			return nil, false
		}
		arg[n] = [2]string{ident(toks, k+1), ident(toks, k+3)}
		k += 4
	}
	if !isPunct(toks, k, ")") || arg[0][0] != arg[1][0] {
		return nil, false
	}
	for i := range toks {
		if w(toks, i) != "VALUES" || i >= at {
			continue
		}
		pairs, c1, c2, ok := valueList(toks, i)
		if !ok || len(pairs) == 0 || c1 != arg[0][1] || c2 != arg[1][1] || forVar(toks, i) != arg[0][0] {
			continue
		}
		return pairs, true
	}
	return nil, false
}

// forVar returns the variable of the closest `FOR x IN` before toks[i].
func forVar(toks []tok, i int) string {
	for k := i - 1; k >= 0; k-- {
		if w(toks, k) == "FOR" && w(toks, k+2) == "IN" {
			return ident(toks, k+1)
		}
	}
	return ""
}

// valueList reads the VALUES list at body[i]: its ('a', 'b') pairs and the two column names of the alias that follows
// `(VALUES ...) AS alias(c1, c2)`. ok is false when a tuple is not a pair of strings or the alias is not there.
func valueList(body []tok, i int) (pairs []string, c1, c2 string, ok bool) {
	j := i + 1
	for isPunct(body, j, "(") {
		if !(j+4 < len(body) && body[j+1].K == kStr && isPunct(body, j+2, ",") && body[j+3].K == kStr && isPunct(body, j+4, ")")) {
			return nil, "", "", false
		}
		pairs = append(pairs, fmt.Sprintf("%s %s", strings.TrimPrefix(body[j+1].U, "public."), body[j+3].U))
		j += 5
		if !isPunct(body, j, ",") {
			break
		}
		j++
	}
	if !isPunct(body, j, ")") {
		return nil, "", "", false
	}
	j++
	if w(body, j) == "AS" {
		j++
	}
	if ident(body, j) == "" || !isPunct(body, j+1, "(") || !isPunct(body, j+3, ",") || !isPunct(body, j+5, ")") {
		return nil, "", "", false
	}
	return pairs, ident(body, j+2), ident(body, j+4), true
}
