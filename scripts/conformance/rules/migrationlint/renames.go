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
	for _, st := range split(lex(src, 1)) {
		if w(st, 0) != "DO" {
			lit(st)
			continue
		}
		body, ok := doBody(st)
		if !ok {
			continue
		}
		dyn := 0
		for _, x := range body {
			if x.K != kStr {
				continue
			}
			switch renameString(x.U) {
			case "table":
				if dyn == 0 {
					dyn = x.Line
				}
			case "bad":
				errs = append(errs, viol{x.Line, "unrecognised RENAME TO inside a string of a DO block"})
			}
		}
		for _, b := range split(body) {
			lit(stripPL(b))
		}
		if dyn > 0 {
			ps := valuePairs(body)
			if len(ps) == 0 {
				errs = append(errs, viol{dyn, "dynamic table rename with no (VALUES ('old', 'new'), ...) list to read the pairs from"})
			}
			for _, p := range ps {
				seen[p] = true
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
// sequence or view rename) or "bad" (anything else).
func renameString(s string) string {
	n := strings.Join(strings.Fields(strings.ToUpper(s)), " ")
	if !strings.Contains(n, "RENAME TO") {
		return ""
	}
	switch {
	case strings.HasPrefix(n, "ALTER TABLE "):
		return "table"
	case strings.HasPrefix(n, "ALTER INDEX "), strings.HasPrefix(n, "ALTER SEQUENCE "), strings.HasPrefix(n, "ALTER VIEW "), strings.HasPrefix(n, "ALTER MATERIALIZED VIEW "):
		return "ok"
	}
	return "bad"
}

// valuePairs reads every ('a', 'b') tuple of two strings that follows a VALUES keyword in body.
func valuePairs(body []tok) []string {
	var out []string
	for i := range body {
		if w(body, i) != "VALUES" {
			continue
		}
		j := i + 1
		for isPunct(body, j, "(") {
			if j+4 < len(body) && body[j+1].K == kStr && isPunct(body, j+2, ",") && body[j+3].K == kStr && isPunct(body, j+4, ")") {
				out = append(out, fmt.Sprintf("%s %s", strings.TrimPrefix(body[j+1].U, "public."), body[j+3].U))
			}
			depth := 0
			for ; j < len(body); j++ {
				if isPunct(body, j, "(") {
					depth++
				} else if isPunct(body, j, ")") {
					if depth--; depth == 0 {
						j++
						break
					}
				}
			}
			if !isPunct(body, j, ",") {
				break
			}
			j++
		}
	}
	return out
}
