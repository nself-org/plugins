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
		case "EXISTS":
			probe = probe || isPunct(body, i+1, "(")
		case "TO_REGCLASS", "TO_REGTYPE", "TO_REGPROC", "TO_REGNAMESPACE", "INFORMATION_SCHEMA", "PG_CATALOG",
			"PG_CLASS", "PG_INDEXES", "PG_CONSTRAINT", "PG_TYPE", "PG_TRIGGER", "PG_POLICIES":
			probe = true
		}
	}
	return branch && probe
}

// protectedStmts marks the statements of a DO body that sit in a BEGIN..END block with an EXCEPTION clause: the handler
// makes a failing replay harmless, so they are not linted.
func protectedStmts(sts [][]tok) []bool {
	type blk struct {
		start int
		exc   bool
	}
	prot := make([]bool, len(sts))
	var stack []blk
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
				stack = append(stack, blk{start: i})
			case x.K == kWord && x.U == "EXCEPTION" && len(stack) > 0:
				top := &stack[len(stack)-1]
				top.exc = true
				for k := top.start; k <= i; k++ {
					prot[k] = true
				}
			}
		}
		for _, b := range stack {
			prot[i] = prot[i] || b.exc
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

// lintDO lints a DO block. Statements inside a BEGIN..END with an EXCEPTION clause are replay-safe. When the block
// branches on an existence probe (ifGuarded), the statements inside IF..END IF are safe too, and so is everything after
// an `IF .. THEN RETURN; END IF;` guard clause; everything else is linted
// with the ordinary rules and may not run dynamic SQL (EXECUTE).
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
	depth, early := 0, false // early: an `IF .. THEN RETURN` guard clause has been seen, the rest of the block is its guarded part
	for i, st := range sts {
		rest, opened := stripPLn(st)
		before, inner := depth, depth+opened
		depth = inner
		if w(st, 0) == "END" && w(st, 1) == "IF" {
			depth--
		}
		if prot[i] {
			continue
		}
		if byIf && inner > 0 && w(rest, 0) == "RETURN" {
			early = true
		}
		dyn := func(ts []tok, depthHere int) {
			if byIf && (depthHere > 0 || early) {
				return
			}
			for _, b := range ts {
				if b.K == kWord && b.U == "EXECUTE" {
					out = append(out, viol{b.Line, "unguarded dynamic SQL: EXECUTE in a DO block outside an EXCEPTION WHEN handler or an existence-checked IF"})
				}
			}
		}
		dyn(st[:len(st)-len(rest)], before)
		dyn(rest, inner)
		if !byIf || (inner == 0 && !early) {
			out = append(out, lintStmt(rest, c)...)
		}
	}
	return out
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
