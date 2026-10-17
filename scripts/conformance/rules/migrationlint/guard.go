// guard.go: the existence guards of a DO block (P7 D-0619 F1). A guard is an IF/ELSIF branch whose own condition probes the
// catalog (EXISTS (..), to_reg*, pg_* names), or the clause that follows `IF <probe> THEN RETURN; END IF;`. A guard covers a
// statement when (1) the probe names an object the statement mentions and (2) the guard's polarity fits the statement: a
// statement that creates or adds needs a "the object is missing" guard, one that drops, renames, alters or updates needs
// "the object exists". A condition with a probe but no clear polarity (a count, a join) fits both. Guards are tracked per
// branch: an unrelated IF, an ELSE, a loop or a CASE branch is not covered by the probe of another IF.
package main

import "strings"

type pol int

const (
	polNeutral pol = iota // probe with no clear polarity
	polExists             // true when the object exists
	polMissing            // true when the object is missing
)

// guard is the probe of one branch: the names it checks and its polarity in the branch that runs when it holds.
type guard struct {
	names map[string]bool
	pol   pol
}

type frame struct {
	kind byte // 'I' IF, 'L' loop, 'C' CASE, 'B' BEGIN block
	g    *guard
}

type guardState struct {
	stack []frame
	early *guard // set after `IF <probe> THEN RETURN; END IF;`: the rest of the block runs when the probe is false
}

type ev struct {
	k    string
	cond []tok
}

// prefixEvents lists the plpgsql scaffolding at the start of a statement, in order (the same words stripPLn skips).
func prefixEvents(t []tok) []ev {
	var out []ev
	for len(t) > 0 {
		switch w(t, 0) {
		case "BEGIN", "LOOP", "ELSE":
			out = append(out, ev{w(t, 0), nil})
			t = t[1:]
		case "DECLARE", "EXCEPTION":
			t = t[1:]
		case "":
			if len(t) < 5 || !isPunct(t, 0, "<") || !isPunct(t, 1, "<") || !isPunct(t, 3, ">") || !isPunct(t, 4, ">") {
				return out
			}
			t = t[5:]
		case "IF", "ELSIF", "ELSEIF", "WHILE", "FOR", "FOREACH", "WHEN", "CASE":
			end, depth, k := "THEN", 0, 1
			if h := w(t, 0); h == "WHILE" || h == "FOR" || h == "FOREACH" {
				end = "LOOP"
			}
			for ; k < len(t); k++ {
				if isPunct(t, k, "(") {
					depth++
				} else if isPunct(t, k, ")") {
					depth--
				} else if depth == 0 && w(t, k) == end {
					break
				}
			}
			if k >= len(t) {
				return out
			}
			out = append(out, ev{w(t, 0), t[1:k]})
			t = t[k+1:]
		default:
			return out
		}
	}
	return out
}

// enter applies the scaffolding of a statement to the frame stack.
func (g *guardState) enter(prefix []tok) {
	for _, e := range prefixEvents(prefix) {
		switch e.k {
		case "BEGIN":
			g.stack = append(g.stack, frame{kind: 'B'})
		case "LOOP", "WHILE", "FOR", "FOREACH":
			g.stack = append(g.stack, frame{kind: 'L'})
		case "CASE":
			g.stack = append(g.stack, frame{kind: 'C'})
		case "IF":
			g.stack = append(g.stack, frame{kind: 'I', g: probeGuard(e.cond, false)})
		case "ELSIF", "ELSEIF":
			if n := len(g.stack); n > 0 && g.stack[n-1].kind == 'I' {
				g.stack[n-1].g = probeGuard(e.cond, false)
			}
		case "ELSE":
			if n := len(g.stack); n > 0 && g.stack[n-1].kind == 'I' {
				g.stack[n-1].g = nil
			}
		}
	}
}

func (g *guardState) pop() {
	if n := len(g.stack); n > 0 {
		g.stack = g.stack[:n-1]
	}
}

func (g *guardState) anyGuard() bool {
	if g.early != nil {
		return true
	}
	for _, f := range g.stack {
		if f.g != nil {
			return true
		}
	}
	return false
}

// earlyReturn recognises the guard clause just entered (statement i is the RETURN inside it): a probe IF directly inside the
// outermost BEGIN..END, whose body is the RETURN alone. The statements after it then run when the probe is false.
func (g *guardState) earlyReturn(sts [][]tok, i int) bool {
	if len(g.stack) != 2 || g.stack[0].kind != 'B' || g.stack[1].kind != 'I' || g.stack[1].g == nil {
		return false
	}
	if i+1 >= len(sts) || len(sts[i+1]) != 2 || w(sts[i+1], 0) != "END" || w(sts[i+1], 1) != "IF" {
		return false
	}
	if g.early == nil {
		in := g.stack[1].g
		g.early = &guard{names: in.names, pol: flip(in.pol)}
	}
	return true
}

func flip(p pol) pol {
	switch p {
	case polExists:
		return polMissing
	case polMissing:
		return polExists
	}
	return polNeutral
}

// covers reports whether a guard in scope covers the statement.
func (g *guardState) covers(st []tok) bool {
	if len(st) == 0 {
		return true
	}
	creates := stmtCreates(st)
	fits := func(x *guard) bool {
		if x == nil || (x.pol == polExists && creates) || (x.pol == polMissing && !creates) {
			return false
		}
		return len(x.names) == 0 || mentions(st, x.names)
	}
	if fits(g.early) {
		return true
	}
	for _, f := range g.stack {
		if fits(f.g) {
			return true
		}
	}
	return false
}

// stmtCreates reports whether a statement adds something (CREATE, ALTER .. ADD, INSERT, or EXECUTE of such a string).
func stmtCreates(st []tok) bool {
	switch w(st, 0) {
	case "CREATE", "INSERT":
		return true
	case "ALTER":
		return contains(st, "ADD")
	case "EXECUTE":
		for _, x := range st {
			if x.K == kStr {
				return stmtCreates(lex(x.U, x.Line))
			}
		}
	}
	return false
}

// mentions reports whether a statement names one of the objects (words, quoted identifiers or string parts).
func mentions(st []tok, names map[string]bool) bool {
	for _, x := range st {
		switch x.K {
		case kWord:
			if names[strings.ToLower(x.U)] {
				return true
			}
		case kQID:
			if names[strings.ToLower(x.U)] {
				return true
			}
		case kStr:
			for _, p := range nameParts(x.U) {
				if names[p] {
					return true
				}
			}
		}
	}
	return false
}

// nameParts splits a string into lower-cased identifier-like parts ("public.t" gives public, t).
func nameParts(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x80)
	})
}

// probeGuard builds the guard of a condition, nil when the condition has no existence probe.
func probeGuard(cond []tok, _ bool) *guard {
	probe := false
	names := map[string]bool{}
	pos, neg := 0, 0
	for i, x := range cond {
		if isProbe(cond, i) {
			probe = true
		}
		switch {
		case x.K == kStr:
			for _, p := range nameParts(x.U) {
				names[p] = true
			}
		case x.K == kQID:
			names[strings.ToLower(x.U)] = true
		case x.K == kWord && (x.U == "FROM" || x.U == "JOIN"):
			if n := ident(cond, i+1); n != "" && !strings.HasPrefix(n, "pg_") && n != "information_schema" {
				names[n] = true
			}
		}
		switch {
		case w(cond, i) == "EXISTS" && isPunct(cond, i+1, "("):
			if w(cond, i-1) == "NOT" {
				neg++
			} else {
				pos++
			}
		case has(cond, i, "IS", "NOT", "NULL"):
			pos++
		case has(cond, i, "IS", "NULL"):
			neg++
		}
	}
	if !probe {
		return nil
	}
	delete(names, "public")
	p := polNeutral
	switch {
	case pos > 0 && neg == 0:
		p = polExists
	case neg > 0 && pos == 0:
		p = polMissing
	}
	return &guard{names: names, pol: p}
}
