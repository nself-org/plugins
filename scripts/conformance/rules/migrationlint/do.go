// do.go: DO block handling of the idempotency lint (P7 D-0346). A DO block is replay-safe where an EXCEPTION handler or an
// existence-checked IF covers its statements; what sits outside is linted like top-level SQL and may not run EXECUTE.
package main

// ifGuarded reports whether a DO body branches (IF) on an existence probe. DDL "IF [NOT] EXISTS name" is not a branch and
// not a probe: EXISTS counts only before "(".
func ifGuarded(body []tok) bool {
	branch, probe := false, false
	for i := range body {
		switch w(body, i) {
		case "IF", "ELSIF", "ELSEIF":
			j := i + 1
			if has(body, j, "NOT", "EXISTS") {
				j++
			}
			if w(body, j) != "EXISTS" || isPunct(body, j+1, "(") {
				branch = true
			}
		default:
			probe = probe || isProbe(body, i)
		}
	}
	return branch && probe
}

// isProbe reports whether t[i] is an existence probe: EXISTS ( ..), a to_reg* call or a catalog name.
func isProbe(t []tok, i int) bool {
	switch w(t, i) {
	case "EXISTS":
		return isPunct(t, i+1, "(")
	case "TO_REGCLASS", "TO_REGTYPE", "TO_REGPROC", "TO_REGNAMESPACE", "INFORMATION_SCHEMA", "PG_CATALOG",
		"PG_CLASS", "PG_INDEXES", "PG_CONSTRAINT", "PG_TYPE", "PG_TRIGGER", "PG_POLICIES":
		return true
	}
	return false
}

// protectedStmts marks the statements that sit in the protected part of a BEGIN..END block, from its BEGIN up to its
// EXCEPTION clause: the handler makes a failing replay harmless. Statements of the handler itself are not protected (an
// error there is not caught by the same block).
func protectedStmts(sts [][]tok) []bool {
	prot := make([]bool, len(sts))
	var stack []int // statement index of each open BEGIN
	for i, st := range sts {
		if w(st, 0) == "END" {
			if n := w(st, 1); n != "IF" && n != "LOOP" && n != "CASE" && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		rest, _ := stripPLn(st)
		for _, x := range st[:len(st)-len(rest)] {
			switch {
			case x.K == kWord && x.U == "BEGIN":
				stack = append(stack, i)
			case x.K == kWord && x.U == "EXCEPTION" && len(stack) > 0:
				for k := stack[len(stack)-1]; k < i; k++ {
					prot[k] = true
				}
			}
		}
	}
	return prot
}

// doBody returns the tokens of a DO statement's body: its $$ body, else its last string literal.
func doBody(t []tok) ([]tok, bool) {
	pick := -1
	for i, x := range t {
		if x.K == kBody {
			return lex(x.U, x.BodyLine), true
		}
		if x.K == kStr {
			pick = i
		}
	}
	if pick < 0 {
		return nil, false
	}
	return lex(t[pick].U, t[pick].Line), true
}

// lintDO lints a DO block. Statements in the protected part of a BEGIN..END with an EXCEPTION clause are replay-safe. When
// the block branches on an existence probe (ifGuarded), the statements inside IF..END IF are safe too, and so is everything
// after a top-level `IF <probe> THEN RETURN; END IF;` guard clause. Everything else (loop, CASE and ELSE bodies included) is
// linted with the ordinary rules and may not run dynamic SQL (EXECUTE).
func lintDO(t []tok, c *ctx) []viol {
	body, ok := doBody(t)
	if !ok {
		return []viol{{t[0].Line, "DO statement with no body to check"}}
	}
	sts := split(body)
	prot, byIf := protectedStmts(sts), ifGuarded(body)
	c.inDO = true
	defer func() { c.inDO = false }()
	var out []viol
	var open pl    // scopes open before the current statement; open.begins counts BEGIN..END blocks
	early := false // a guard clause was seen: the rest of the block is its guarded part
	for i, st := range sts {
		rest, n := stripPLn(st)
		prefix := st[:len(st)-len(rest)]
		before, inner := open.ifs, open.ifs+n.ifs
		if byIf && !early && guardReturn(sts, i, prefix, rest, n, open) {
			early = true
		}
		open.ifs, open.loops, open.cases, open.begins = open.ifs+n.ifs, open.loops+n.loops, open.cases+n.cases, open.begins+n.begins
		if w(st, 0) == "END" {
			switch w(st, 1) {
			case "IF":
				open.ifs--
			case "LOOP":
				open.loops--
			case "CASE":
				open.cases--
			default:
				open.begins--
			}
		}
		if prot[i] {
			continue
		}
		dyn := func(ts []tok, ifs int) {
			if byIf && (ifs > 0 || early) {
				return
			}
			for _, b := range ts {
				if b.K == kWord && b.U == "EXECUTE" {
					out = append(out, viol{b.Line, "unguarded dynamic SQL: EXECUTE in a DO block outside an EXCEPTION WHEN handler or an existence-checked IF"})
				}
			}
		}
		dyn(prefix, before)
		dyn(rest, inner)
		if !byIf || (inner == 0 && !early) {
			out = append(out, lintStmt(rest, c)...)
		}
	}
	return out
}

// guardReturn reports whether statement i is `IF <cond with an existence probe> THEN RETURN` whose IF body holds nothing
// else (the next statement is END IF) and which sits directly in the outermost BEGIN..END: not in another IF, loop, CASE
// or nested block.
func guardReturn(sts [][]tok, i int, prefix, rest []tok, n pl, open pl) bool {
	if n.ifs != 1 || n.loops != 0 || n.cases != 0 || open.ifs != 0 || open.loops != 0 || open.cases != 0 || open.begins+n.begins != 1 {
		return false
	}
	if len(rest) != 1 || w(rest, 0) != "RETURN" || i+1 >= len(sts) || len(sts[i+1]) != 2 || w(sts[i+1], 0) != "END" || w(sts[i+1], 1) != "IF" {
		return false
	}
	k := 0
	for w(prefix, k) == "BEGIN" || w(prefix, k) == "DECLARE" {
		k++
	}
	if w(prefix, k) != "IF" {
		return false
	}
	for j := k; j < len(prefix); j++ {
		if isProbe(prefix, j) {
			return true
		}
	}
	return false
}

// selectInto reports a top-level SELECT ... INTO <table> (not inside parentheses).
func selectInto(t []tok) bool {
	depth := 0
	for i := range t {
		switch {
		case isPunct(t, i, "("):
			depth++
		case isPunct(t, i, ")"):
			depth--
		case depth == 0 && w(t, i) == "INTO":
			return true
		}
	}
	return false
}
